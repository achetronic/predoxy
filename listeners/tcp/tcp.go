package tcp

// Ref: https://scene-si.org/2020/05/29/waiting-on-goroutines/
// Ref: https://gist.github.com/jbardin/821d08cb64c01c84b81a

import (
	"github.com/achetronic/predoxy/api"
	"io"
	"net"
	"time"
)

const (
	ProtocolTcp                  = "tcp"
	ConnectionResponseBufferSize = 1024 * 1024 // 200KB
	ConnectionTimeoutSeconds     = 10 * 60     // 10 minutes

	// Error messages
	FailedReadingConnectionErrorMessage  = "Error reading from connection socket: %q"
	FailedWritingConnectionErrorMessage  = "Error writing to the connection socket: %q"
	FailedProcessingPipelineErrorMessage = "Error parsing the message: %q"

	FailedConnectionDataParsingErrorMessage = "Error parsing the host and port from remote: %q"
	FailedClosingConnectionErrorMessage     = "Error closing the connection on socket: %q"

	// Info messages
	ClientConnectedInfoMessage = "A new connection established from remote: %s"

	// Debug messages
	ConnectionClosedDebugMessage = "Connection was closed"
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

	return backendConn, err
}

// readAll buffer each message from the source connection until EOF, and return the whole message
func (p *TCPProxy) readAllFromConnection(sourceConn *net.Conn) (message []byte, err error) {

	// Exchange buffer
	tmpBuffer := make([]byte, 250)

	// Buffer for appending the chunks until EOF
	buffer := make([]byte, 0, ConnectionResponseBufferSize)

	var nRead int

	for {
		nRead, err = (*sourceConn).Read(tmpBuffer[:cap(tmpBuffer)])
		if err != nil {
			if err != io.EOF {
				return message, err
			}
			break
		}

		buffer = append(buffer, tmpBuffer[:nRead]...)

		if nRead < cap(tmpBuffer) {
			break
		}
	}

	return buffer, err
}

// forwardPackets forwards packets from source to destination, executing a callback in the middle to process messages
// This function will be executed as a goroutine
func (p *TCPProxy) forwardPackets(
	sourceConn net.Conn,
	destConn net.Conn,
	sourceConnClosed chan struct{},
	callback ForwardCallback) {

	var message []byte
	var err error

	// Create params structure to pass to the callback
	callbackParameters := ForwardCallbackParams{
		SourceConnection: &sourceConn,
		DestConnection:   &destConn,
		Cache:            p.Cache,
		Message:          &message,
	}

	for {

		// Read the whole message from the source
		message, err = p.readAllFromConnection(&sourceConn)
		if err != nil {
			p.Logger.Errorf(FailedReadingConnectionErrorMessage, err)
			break
		}

		// Execute the pipeline for the message
		message, err = callback(&callbackParameters)
		if err != nil {
			p.Logger.Errorf(FailedProcessingPipelineErrorMessage, err)
			break
		}

		// Write the response to the client
		_, err = destConn.Write(message)
		if err != nil {
			p.Logger.Errorf(FailedWritingConnectionErrorMessage, err)
			break
		}

		// Reset the message
		message = []byte{}
	}

	if err := sourceConn.Close(); err != nil {
		p.Logger.Errorf(FailedClosingConnectionErrorMessage, err)
	}
	sourceConnClosed <- struct{}{}

	p.Logger.Debug(ConnectionClosedDebugMessage)
}

//// forwardCommandPackets forwards TCP packets from a source stream to destination steam, converting it into transaction
//// This function will be executed as a goroutine
//func (p *TCPProxy) forwardCommandPackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {
//
//	var message []byte
//	var err error
//
//	for {
//
//		// Read the whole message from the source
//		message, err = p.readAllFromConnection(&sourceConn)
//		if err != nil {
//			p.Logger.Errorf(FailedReadingResponseErrorMessage, err)
//			break
//		}
//
//		// Construct a transaction message from the wrapper
//		transactionMessage := MessageTransaction
//		transactionMessage[2] = message
//
//		// Parse the request to lazy update cached db index
//		// TODO: Improve the performance of this logic
//		p.setCachedDB(&sourceConn, &message)
//		dbIndex, _ := p.getCachedDB(&sourceConn)
//		transactionMessage[1] = []byte(fmt.Sprintf(CommandSelect, dbIndex))
//
//		p.Logger.Debugf(QueryBeforeFilterDebugMessage, string(message))
//
//		// Modify the request according to the filters
//		p.filterCommands(&message, &transactionMessage)
//		p.Logger.Debugf(QueryAfterFilterDebugMessage, string(transactionMessage[2]))
//
//		// Convert the message representation into a bytes before sending
//		modifiedChunk := bytes.Join(transactionMessage, []byte{})
//		log.Print(string(modifiedChunk))
//
//		// Write the response to the client
//		_, err = destConn.Write(message)
//		if err != nil {
//			p.Logger.Errorf(FailedWritingQueryErrorMessage, err)
//			break
//		}
//
//		// Reset the message
//		message = []byte{}
//	}
//
//	if err := sourceConn.Close(); err != nil {
//		p.Logger.Errorf(FailedClosingClientConnectionErrorMessage, err)
//	}
//	sourceConnClosed <- struct{}{}
//
//	p.Logger.Debug(ClientConnectionClosedDebugMessage)
//}
//
//// forwardResponsePackets forwards TCP packets from a source stream to destination steam, offloading transaction
//// This function will be executed as a goroutine
//func (p *TCPProxy) forwardResponsePackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {
//
//	var message []byte
//	var err error
//
//	for {
//
//		// Read the whole message from the source
//		message, err = p.readAllFromConnection(&sourceConn)
//		if err != nil {
//			p.Logger.Errorf(FailedReadingResponseErrorMessage, err)
//			break
//		}
//
//		// Write the response to the client
//		_, err = destConn.Write(message)
//		if err != nil {
//			p.Logger.Errorf(FailedWritingResponseErrorMessage, err)
//			break
//		}
//
//		// Reset the message
//		message = []byte{}
//	}
//
//	// Send the right response to the user
//	if err := sourceConn.Close(); err != nil {
//		p.Logger.Errorf(FailedClosingServerConnectionErrorMessage, err)
//	}
//	sourceConnClosed <- struct{}{}
//
//	p.Logger.Debug(ServerConnectionClosedDebugMessage)
//}

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

	frontendClosed := make(chan struct{}, 1) // cliConn
	backendClosed := make(chan struct{}, 1)  // srvConn

	// Send the request from the frontend to the backend server
	//go p.forwardCommandPackets(frontendConn, backendConn, frontendClosed)
	go p.forwardPackets(frontendConn, backendConn, frontendClosed, p.incomingMessagesPipeline)

	// Send the response back to frontend contacting us
	//go p.forwardResponsePackets(backendConn, frontendConn, backendClosed)
	go p.forwardPackets(backendConn, frontendConn, backendClosed, p.outgoingMessagesPipeline)

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

	// Init the cache for this proxy
	(*p.Cache).ConnectionPool = make(api.ConnectionPoolMap)

	// Resolve frontend IP address from config
	frontendHost, err := getTCPAddress(p.Config.Listener.Host, p.Config.Listener.Port)
	if err != nil {
		p.Logger.Errorf(FailedConnectionDataParsingErrorMessage, err)
		return err
	}

	// Listen for incoming connections.
	frontendServer, err := net.ListenTCP(ProtocolTcp, frontendHost)
	if err != nil {
		return err
	}

	// Set the timeouts for the connections
	// TODO: decide the deadline policy
	frontendServer.SetDeadline(time.Now().Add(ConnectionTimeoutSeconds * time.Second))

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

		p.Logger.Infof(ClientConnectedInfoMessage, frontendConn.RemoteAddr().String())
	}
}
