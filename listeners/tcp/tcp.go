package tcp

// Ref: https://scene-si.org/2020/05/29/waiting-on-goroutines/
// Ref: https://gist.github.com/jbardin/821d08cb64c01c84b81a

import (
	"bufio"
	"bytes"
	"github.com/achetronic/redis-proxy/api"
	"io"
	"log"
	"net"
	"os"
)

const (
	ProtocolTcp = "tcp"
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

// forwardPackets forwards TCP packets from a source stream to destination steam
// This function will be executed as a goroutine
func (p *TCPProxy) forwardPackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {

	_, err := io.Copy(destConn, sourceConn)

	if err != nil {
		log.Printf("Copy error: %s", err)
	}
	if err := sourceConn.Close(); err != nil {
		log.Printf("Close error: %s", err)
	}
	sourceConnClosed <- struct{}{}
}

// forwardEncodedPackets forwards TCP packets from a source stream to destination steam, modifying them on the fly
// This function will be executed as a goroutine
func (p *TCPProxy) forwardEncodedPackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {

	buf := new(bytes.Buffer)

	// Now create a reader which will read from the buffer, and use it to read
	// the streamed array.
	br := bufio.NewReader(buf)

	// 1. Analyze packets looking for select clause
	// 2. If select found, add registry to cache
	// 3. If select not found try to get it from memory (or set)
	// 4. Modify the content to craft a transaction
	// 5. Send the transaction
	log.Println("Entramos en el forwardEncodedPackets")

	// Create a multi reader
	mr := io.MultiReader(sourceConn)

	// Create a multi writer
	mw := io.MultiWriter(destConn, os.Stdout, br)

	_, err := io.Copy(mw, mr)
	if err != nil {
		log.Printf("Copy error: %s", err)
	}

	if err := sourceConn.Close(); err != nil {
		log.Printf("Close error: %s", err)
	}
	sourceConnClosed <- struct{}{}

	log.Println("Salimos en el forwardEncodedPackets")
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
	go p.forwardPackets(backendConn, frontendConn, backendClosed)

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
