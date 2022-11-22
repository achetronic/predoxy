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
)

const (
	ProtocolTcp = "tcp"

	// Definitions of common used commands
	CommandCommandDocs = "*2\r\n$7\r\nCOMMAND\r\n$4\r\nDOCS\r\n"
	CommandMulti       = "*1\r\n$5\r\nMULTI\r\n"
	CommandSelect      = "*2\r\n$6\r\nSELECT\r\n$1\r\n%d\r\n"
	CommandExec        = "*1\r\n$4\r\nEXEC\r\n"
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

// forwardDecodedPackets forwards TCP packets from a source stream to destination steam, offloading transaction
// This function will be executed as a goroutine
func (p *TCPProxy) forwardDecodedPackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {

	// Small temporary buffer
	tmp := make([]byte, 256)

	// Big exchange buffer
	buffer := make([]byte, 0, 4096)

	chunk := make([]byte, 0, 4096)

	for {
		// Read each message from the source
		n, err := sourceConn.Read(tmp)
		if err != nil {
			if err != io.EOF {
				fmt.Println("read error:", err)
			}
			break
		}

		//
		chunk = append(buffer, tmp[:n]...)
		log.Printf("Response chunk: %q", string(chunk[0:n]))

		var responseBytes []byte

		// Parse the chunk to transform transaction into a single command request
		// Ref: https://pkg.go.dev/github.com/tidwall/resp#section-readme
		respReader := resp.NewReader(bytes.NewBufferString(string(chunk[0:n])))
		for {
			v, _, err := respReader.ReadValue()
			if err == io.EOF {
				break
			}
			if err != nil {
				(*p.Logger).Write([]byte(err.Error()))
			}

			// MULTI command always answers with arrays
			if v.Type() == resp.Array {

				responseBytes, _ = v.MarshalRESP()

				// Arrays of 0 are unexpected errors
				if len(v.Array()) == 0 {
					parsedResponse := v.Error().Error()
					responseBytes = []byte(parsedResponse)
				}

				// Arrays of 1 item are just strings
				if len(v.Array()) == 1 {
					parsedResponse := v.Array()[0]
					responseBytes, _ = parsedResponse.MarshalRESP()
				}
			}
		}

		//log.Printf(string(responseBytes))

		_, err = destConn.Write(responseBytes)
		if err != nil {
			(*p.Logger).Write([]byte(err.Error()))
		}
	}

	// Send the right response to the user
	if err := sourceConn.Close(); err != nil {
		log.Printf("Close error: %s", err)
	}
	sourceConnClosed <- struct{}{}

	log.Println("Salimos en el forwardPackets")
}

// forwardEncodedPackets forwards TCP packets from a source stream to destination steam, converting it into transaction
// This function will be executed as a goroutine
func (p *TCPProxy) forwardEncodedPackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {

	// Small temporary buffer
	tmp := make([]byte, 256)

	// Big exchange buffer
	buffer := make([]byte, 0, 4096)

	chunk := make([]byte, 0, 4096)

	for {
		// Store into a buffer each read byte from connection until finish
		n, err := sourceConn.Read(tmp)
		if err != nil {
			if err != io.EOF {
				fmt.Println("read error:", err)
			}
			break
		}

		// Join all read bytes into a bigger buffer
		chunk = append(buffer, tmp[:n]...)

		// Construct a transaction message
		message := [][]byte{
			[]byte(CommandMulti),
			[]byte(fmt.Sprintf(CommandSelect, 0)),
			[]byte(CommandExec),
		}

		// Ignore COMMAND DOCS command, too heavy response
		// TODO: ALLOW LARGE RESPONSES TO REMOVE THIS CONDITION
		if string(chunk) != CommandCommandDocs {
			message = [][]byte{
				[]byte(CommandMulti),
				[]byte(fmt.Sprintf(CommandSelect, 0)),
				chunk[0:n],
				[]byte(CommandExec),
			}
		}

		// Convert the message into a huge slice of bytes to be sent
		modifiedChunk := bytes.Join(message, []byte{})

		log.Printf("Received Chunk: %q", string(chunk))
		log.Printf("Modified Chunk: %q", string(modifiedChunk))

		// Forward the command to the backend
		_, err = destConn.Write(modifiedChunk)
		if err != nil {
			log.Println(err)
		}
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
		backendConn.SetLinger(0)
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
