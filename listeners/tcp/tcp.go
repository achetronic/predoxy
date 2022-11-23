package tcp

// Ref: https://scene-si.org/2020/05/29/waiting-on-goroutines/
// Ref: https://gist.github.com/jbardin/821d08cb64c01c84b81a

import (
	"bytes"
	"fmt"
	"github.com/achetronic/redis-proxy/api"
	"github.com/tidwall/resp"
	"io"
	"log"
	"net"
	"regexp"
	"time"
)

const (
	ProtocolTcp                  = "tcp"
	ConnectionResponseBufferSize = 8 * 1024 // 8MB
	ConnectionTimeoutSeconds     = 10 * 60  // 10 minutes

	// Definitions of common used commands
	CommandMulti      = "*1\r\n$5\r\nMULTI\r\n"
	CommandSelect     = "*2\r\n$6\r\nSELECT\r\n$1\r\n%d\r\n"
	CommandExec       = "*1\r\n$4\r\nEXEC\r\n"
	CommandNotallowed = "*1\r\n$10\r\nNOTALLOWED\r\n"

	// Regex patterns for complex commands
	RegexQueryCommand          = `(\*[1-9]+(\r?\n)+\$7(\r?\n)+(COMMAND|command)(\r?\n)+){1}(\$[1-9]+(\r?\n)+[A-Za-z]+(\r?\n)+)*`
	RegexQueryPipelinedCommand = `(COMMAND|command){1}((\s?)+[A-Za-z]+)*(\r?\n)*`
	RegexQueryMulti            = `(\*[1-9]+(\r?\n)+\$5(\r?\n)+(MULTI|multi)(\r?\n)+){1}(\$[1-9]+(\r?\n)+[A-Za-z]+(\r?\n)+)*`
	RegexQueryPipelinedMulti   = `(MULTI|multi){1}((\s?)+[A-Za-z]+)*(\r?\n)*`

	// Error messages
	FailedWritingResponseErrorMessage = "Error writing the response: %q"
	FailedParsingResponseErrorMessage = "Error parsing the response: %q"
	FailedReadingResponseErrorMessage = "Error reading the response: %q"
)

var (

	// MessageTransaction represents a wrapper for the actual message
	MessageTransaction = [][]byte{
		[]byte(CommandMulti),
		[]byte(fmt.Sprintf(CommandSelect, 0)),
		{},
		[]byte(CommandExec),
	}

	// QueryFilters groups the filters which will be applied to requested queries before executing them
	QueryFilters = []string{
		// TODO: Process MULTI commands. Do we want to support nested MULTI?
		RegexQueryMulti,
		RegexQueryPipelinedMulti,
		// TODO: Allow large responses to remove COMMAND related restrictions
		RegexQueryCommand,
		RegexQueryPipelinedCommand,
	}
)

// createBackendConnection create a connection to the given backend on ConnectionPoolTCP,
// and returns a pointer to the connection and its id
func (p *TCPProxy) createBackendConnection(backendConfig *api.Backend) (backendConn *net.TCPConn, err error) {

	// Resolve the address for the given backend
	backendHost, err := getTCPAddress(backendConfig.Host, backendConfig.Port)
	if err != nil {
		return backendConn, err
	}

	// Open a connection with a remote server
	backendConn, err = net.DialTCP(ProtocolTcp, nil, backendHost)
	if err != nil {
		return backendConn, err
	}

	// Initialize the ConnectionPoolTCP of the backend when empty
	/*if (*p.Cache).ConnectionPool == nil {
		(*p.Cache).ConnectionPool = make(api.ConnectionPoolMap)
	}*/

	return backendConn, err
}

// deleteBackendConnection delete a connection from the ConnectionPoolTCP
//func (p *TCPProxy) deleteBackendConnection(backend *api.Backend, connectionId uint64) {
//	delete((*p.Cache)[backend.Id].ConnectionPoolTCP, connectionId)
//}

// parseRespResponse parse the response buffer to transform transaction into a single command request
// Ref: https://pkg.go.dev/github.com/tidwall/resp#section-readme
func (p *TCPProxy) parseRespResponse(byteString []byte) (responseBytes []byte, err error) {

	respReader := resp.NewReader(bytes.NewBufferString(string(byteString)))
	for {
		v, _, err := respReader.ReadValue()
		if err == io.EOF {
			break
		}
		if err != nil {
			return responseBytes, err
		}

		log.Printf("Mostramos toda la respuesta parseada: %q", v)

		// Forward RESP errors
		if v.Type() == resp.Error {
			log.Printf("Hubo un error en la respuesta %q", v.Error().Error())
			parsedResponse := resp.ErrorValue(v.Error())
			responseBytes, _ = parsedResponse.MarshalRESP()
			break
		}

		// Parse arrays: MULTI command always answers with arrays
		if v.Type() == resp.Array {

			// Arrays of 1 item must be treated just as strings
			if len(v.Array()) == 1 {
				parsedResponse := v.Array()[0]
				responseBytes, _ = parsedResponse.MarshalRESP()
			}

			// The most common case for single commands
			if len(v.Array()) == 2 {
				parsedResponse := v.Array()[1]
				responseBytes, _ = parsedResponse.MarshalRESP()
			}

			// Pipelining detected, processing all the responses
			if len(v.Array()) > 2 {
				for index, item := range v.Array() {
					if index == 0 {
						continue
					}
					itemBytes, _ := item.MarshalRESP()
					responseBytes = append(responseBytes, itemBytes...)
				}
			}
		}
	}

	return responseBytes, err
}

// forwardDecodedPackets forwards TCP packets from a source stream to destination steam, offloading transaction
// This function will be executed as a goroutine
func (p *TCPProxy) forwardDecodedPackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {

	// Big exchange buffer
	buffer := make([]byte, 0, ConnectionResponseBufferSize)

	var responseBytes []byte

	for {
		// Read each message from the source
		n, err := sourceConn.Read(buffer[:cap(buffer)])
		if err != nil {
			if err != io.EOF {
				(*p.Logger).Write([]byte(fmt.Sprintf(FailedReadingResponseErrorMessage, err)))
			}
			break
		}

		buffer = buffer[:n]
		log.Printf("Response chunk: %q", string(buffer))

		// Parse the chunk to transform transaction into a single command request
		// Ref: https://pkg.go.dev/github.com/tidwall/resp#section-readme
		responseBytes, err = p.parseRespResponse(buffer)
		if err != nil {
			(*p.Logger).Write([]byte(fmt.Sprintf(FailedParsingResponseErrorMessage, err)))
		}

		_, err = destConn.Write(responseBytes)
		if err != nil {
			(*p.Logger).Write([]byte(fmt.Sprintf(FailedWritingResponseErrorMessage, err)))
		}

		// Clear last read content
		buffer = make([]byte, 0, ConnectionResponseBufferSize)
	}

	// Send the right response to the user
	if err := sourceConn.Close(); err != nil {
		log.Printf("Close error: %s", err)
	}
	sourceConnClosed <- struct{}{}
}

// forwardEncodedPackets forwards TCP packets from a source stream to destination steam, converting it into transaction
// This function will be executed as a goroutine
func (p *TCPProxy) forwardEncodedPackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {

	// Big exchange buffer
	buffer := make([]byte, 0, ConnectionResponseBufferSize)

	for {
		// Store into a buffer each read byte from connection until finish
		n, err := sourceConn.Read(buffer[:cap(buffer)])
		if err != nil {
			if err != io.EOF {
				fmt.Println("read error:", err)
			}
			break
		}

		// Join all read bytes into a bigger buffer
		buffer = buffer[:n]

		// Construct a transaction message from the wrapper
		message := MessageTransaction
		message[2] = buffer

		// Filter several queries by comparing them against patterns
		for _, filter := range QueryFilters {
			matchCommandDocs, _ := regexp.Match(filter, buffer)
			if matchCommandDocs {
				message[2] = []byte(CommandNotallowed)
			}
		}

		// Convert the message representation into a bytes before sending
		modifiedChunk := bytes.Join(message, []byte{})

		log.Printf("Received Chunk: %q", string(buffer))
		log.Printf("Modified Chunk: %q", string(modifiedChunk))

		// Forward the command to the backend
		_, err = destConn.Write(modifiedChunk)
		if err != nil {
			log.Println(err)
		}

		// Clear last read content
		buffer = make([]byte, 0, ConnectionResponseBufferSize)
	}

	if err := sourceConn.Close(); err != nil {
		log.Printf("Close error: %s", err)
	}
	sourceConnClosed <- struct{}{}
}

// handleRequest handle incoming requests, from given frontend to given backend server
// This function will be executed as a goroutine
func (p *TCPProxy) handleRequest(frontendConn *net.TCPConn) {

	// Generate a connection to a backend server for each incoming request
	backendConn, err := p.createBackendConnection(&p.Config.Backend)
	if err != nil {
		// TODO decide how to handle this error because they are inside a goroutine
		println("Error getting remote connection: ", err.Error())
		return
	}

	// Delete this connection once finished execution
	defer func() {
		backendConn.Close()
	}()

	//log.Print(frontendConn.RemoteAddr().String())
	//log.Print(generateConnectionPoolKey(frontendConn))
	//
	//// Create a connection track key for the new connection
	//fromKey := generateConnectionPoolKey(from)
	//(*p.Cache)[nextBackend.Id].CacheLock.Lock()
	//
	//// Look for the connection in the ConnectionPoolUDP table for selected backend
	//backendConn, hit := (*p.Cache).ConnectionPool[*fromKey]

	frontendClosed := make(chan struct{}, 1) // cliConn
	backendClosed := make(chan struct{}, 1)  // srvConn

	// Set the timeouts for the connections
	// TODO: decide the deadline policy
	frontendConn.SetDeadline(time.Time{})
	backendConn.SetDeadline(time.Time{})
	//backendConn.SetDeadline(time.Now().Add(time.Second * ConnectionTimeoutSeconds))

	// Send the request from the frontend to the backend server
	go p.forwardEncodedPackets(frontendConn, backendConn, frontendClosed)

	// Send the response back to frontend contacting us.
	go p.forwardDecodedPackets(backendConn, frontendConn, backendClosed)

	// wait for one half of the proxy to exit, then trigger a shutdown of the
	// other half by calling CloseRead(). This will break the read loop in the
	// broker and allow us to fully close the connection cleanly without a
	// "use of closed network connection" error.
	var waitFor chan struct{}

	select {

	case <-frontendClosed:
		// The client (browser, curl, whatever) closed first and the packets from the backend
		// are not useful anymore, so close the connection with the backend using SetLinger(0) to
		// recycle the port faster
		backendConn.SetLinger(0) // TODO: decide the policy
		backendConn.CloseRead()
		waitFor = backendClosed

	case <-backendClosed:
		// Backend server closed first. This means backend could be down unexpectedly,
		// so close the connection to let the user try again
		frontendConn.CloseRead()
		waitFor = frontendClosed
	}

	// Wait for the other connection to close.
	// This "waitFor" pattern isn't required, but gives us a way to track the
	// connection and ensure all copies terminate correctly; we can trigger
	// stats on entry and deferred exit of this function.
	<-waitFor
}

// Launch start a listener loop to forward traffic to backend servers
func (p *TCPProxy) Launch() (err error) {

	// Resolve frontend IP address from config
	frontendHost, err := getTCPAddress(p.Config.Listener.Host, p.Config.Listener.Port)
	if err != nil {
		log.Printf("error en el getTCPAddres: %s", err)
		return err
	}

	// Listen for incoming connections.
	frontendServer, err := net.ListenTCP(ProtocolTcp, frontendHost)
	if err != nil {
		return err
	}

	// Close the listener when the application closes
	defer frontendServer.Close()

	// Handle incoming connections
	for {

		// Listen for an incoming connection.
		frontendConn, err := frontendServer.AcceptTCP()
		if err != nil {
			return err
		}

		// Process connections in a new goroutine to parallelize them
		go p.handleRequest(frontendConn)
	}
}

// Close TODO: implement the function
func (p *TCPProxy) Close() (err error) {
	return err
}
