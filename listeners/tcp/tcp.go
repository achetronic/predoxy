package tcp

// Ref: https://scene-si.org/2020/05/29/waiting-on-goroutines/
// Ref: https://gist.github.com/jbardin/821d08cb64c01c84b81a

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/achetronic/redis-proxy/api"
	"github.com/tidwall/resp"
	"io"
	"log"
	"net"
	"regexp"
	"strconv"
	"time"
)

const (
	ProtocolTcp                  = "tcp"
	ConnectionResponseBufferSize = 8 * 1024 // 8MB
	ConnectionTimeoutSeconds     = 10 * 60  // 10 minutes
	DefaultDbIndex               = 0

	// Definitions of common individual commands (used to craft commands)
	CommandMulti     = "*1\r\n$5\r\nMULTI\r\n"
	CommandSelect    = "*2\r\n$6\r\nSELECT\r\n$1\r\n%d\r\n"
	CommandExec      = "*1\r\n$4\r\nEXEC\r\n"
	CommandEchoError = "*2\r\n$4\r\nECHO\r\n$8\r\nPERR#%d\r\n"

	// Definitions of common pipelined commands (used to craft commands)
	PipelinedCommandEchoError = "ECHO PERR#%d\r\n"

	// Regex patterns for individual intercepted commands
	RegexQueryCommand = `(\*[1-9]+(\r?\n)\$7(\r?\n)(?i)(command)(\r?\n)){1}(\$[1-9]+(\r?\n)[A-Za-z]+(\r?\n))*`
	RegexQueryMulti   = `(\*[1-9]+(\r?\n)\$5(\r?\n)(?i)(multi)(\r?\n)){1}`
	RegexQuerySelect  = `(\*[1-9]+(\r?\n)\$6(\r?\n)(?i)(select)(\r?\n)){1}(\$[1-9]+(\r?\n)(?P<database>[0-9]+)(\r?\n)){1}`

	// Regex patterns for intercepted pipelined commands
	RegexQueryPipelinedCommand = `(?m)^(?i)(command){1}([[:blank:]]*[A-Za-z]+)*`
	RegexQueryPipelinedMulti   = `(?m)^(?i)(multi){1}([[:blank:]]*[A-Za-z]+)*`
	RegexQueryPipelinedSelect  = `(?m)^(?i)(select){1}([[:blank:]]*(?P<database>[0-9]+)){1}`

	// Regex patterns for intercepted responses
	RegexResponseEchoError          = `(\*[1-9]+(\r?\n)\$4(\r?\n)(?i)(echo)(\r?\n)){1}(\$[1-9]+(\r?\n)(?P<errorcode>[0-9]+)(\r?\n)){1}`
	RegexResponsePipelinedEchoError = `(?m)^(?i)(echo){1}([[:blank:]]*(?P<errorcode>[0-9]+)){1}`

	// Error messages
	FailedWritingResponseErrorMessage = "Error writing the response: %q"
	FailedParsingResponseErrorMessage = "Error parsing the response: %q"
	FailedReadingResponseErrorMessage = "Error reading the response: %q"
)

var (

	// MessageTransaction represents a transaction wrapper for the actual message
	MessageTransaction = [][]byte{
		[]byte(CommandMulti),
		[]byte(fmt.Sprintf(CommandSelect, 0)),
		{},
		[]byte(CommandExec),
	}

	// QueryFilters groups the filters which will be applied to requested queries before executing them
	QueryFilters = []string{
		RegexQueryMulti,
		RegexQueryCommand, // TODO: Allow large responses to remove COMMAND related restrictions
	}

	// PipelinedQueryFilters groups the filters which will be applied to requested queries before executing them
	// ATTENTION: pipelined queries have not a well-defined format, so they have to be filtered after common flow
	PipelinedQueryFilters = []string{
		RegexQueryPipelinedMulti,
		RegexQueryPipelinedCommand,
	}

	// ProxyErrorCodes represents response error codes and its messages
	// As a convention, we use HTTP error codes for ease
	// Ref: https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#client_error_responses
	ProxyErrorCodes = map[int]string{
		403: "forbidden: command is disabled on the proxy",
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

// getCachedDB returns the index of db for selected connection when already cached
func (p *TCPProxy) getCachedDB(sourceConn *net.Conn) (db int, err error) {

	// Assume default db index
	db = DefaultDbIndex

	fromKey, _ := generateConnectionPoolKey(sourceConn)
	(*p.Cache).CacheLock.Lock()
	connCache, hit := (*p.Cache).ConnectionPool[*fromKey]
	(*p.Cache).CacheLock.Unlock()
	if !hit {
		err = errors.New("database not cached for this source connection")
		return db, err
	}

	db = connCache.RedisSelectedDB
	return db, err
}

// setCachedDB TODO
func (p *TCPProxy) setCachedDB(sourceConn *net.Conn, chunk *[]byte) (database int, err error) {

	database = DefaultDbIndex

	// Group all patterns to look for SELECT clause
	selectPatterns := []string{
		RegexQuerySelect,          // SELECT clause into individual queries
		RegexQueryPipelinedSelect, // SELECT clause into pipelined queries
	}

	// Enabling flag to store into cache only when needed
	selectFound := false

	fromKey, _ := generateConnectionPoolKey(sourceConn)

	// Find SELECT clause into the chunk by comparing against several regex patterns
	for _, selectPattern := range selectPatterns {
		compiledSelect := regexp.MustCompile(selectPattern)
		matchSelect := compiledSelect.Match(*chunk)
		if matchSelect {
			//
			selectFound = true

			namedGroups := getRegexNamedGroups(compiledSelect, *chunk)

			// Casting database index in bytes into int
			// TODO: This seems a bit tricky, but iteration over conversion from byteArray to int is needed
			database, err = strconv.Atoi(string(namedGroups[len(namedGroups)-1]["database"]))
			if err != nil {
				return database, err
			}
		}
	}

	if !selectFound {
		return -1, err
	}

	// Store it into cache
	(*p.Cache).CacheLock.Lock()
	(*p.Cache).ConnectionPool[*fromKey] = api.ConnectionTrackStorage{
		RedisSelectedDB: database,
	}
	(*p.Cache).CacheLock.Unlock()

	log.Printf("Cached database: %d", database)

	return database, err
}

// filterCommands update the transaction message by filtering commands from inside the chunk
func (p *TCPProxy) filterCommands(chunk *[]byte, transactionMessage *[][]byte) {

	// Assume commands are always individual
	selectedFilters := QueryFilters
	errorPattern := []byte(fmt.Sprintf(CommandEchoError, 403))

	// Switch to assume we have pipelined commands when the query does NOT start with '*'
	if startingByte := (*chunk)[0]; startingByte != []byte("*")[0] {
		selectedFilters = PipelinedQueryFilters
		errorPattern = []byte(fmt.Sprintf(PipelinedCommandEchoError, 403))
	}

	// Loop over selected filters modifying the chunk
	for _, filter := range selectedFilters {
		compiledFilter := regexp.MustCompile(filter)
		matchFilter := compiledFilter.Match(*chunk)
		if matchFilter {
			(*transactionMessage)[2] = compiledFilter.ReplaceAllLiteral((*transactionMessage)[2], errorPattern)
		}
	}
}

// filterResponse parse the response buffer to transform transaction into a single command request
// Ref: https://pkg.go.dev/github.com/tidwall/resp#section-readme
func (p *TCPProxy) filterResponse(byteString []byte) (responseBytes []byte, err error) {

	log.Printf("%q\n", string(byteString))

	respReader := resp.NewReader(bytes.NewBufferString(string(byteString)))
	for {
		v, _, err := respReader.ReadValue()
		if err == io.EOF {
			break
		}
		if err != nil {
			return responseBytes, err
		}

		// Forward RESP errors
		if v.Type() == resp.Error {
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

// forwardCommandPackets forwards TCP packets from a source stream to destination steam, converting it into transaction
// This function will be executed as a goroutine
func (p *TCPProxy) forwardCommandPackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {

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
		transactionMessage := MessageTransaction
		transactionMessage[2] = buffer

		// Parse the request to lazy update cached db index
		// TODO: Improve the performance of this logic
		p.setCachedDB(&sourceConn, &buffer)
		dbIndex, _ := p.getCachedDB(&sourceConn)
		transactionMessage[1] = []byte(fmt.Sprintf(CommandSelect, dbIndex))

		// Modify the request according to the filters
		log.Printf("Remote: %q", sourceConn.RemoteAddr().String())
		log.Printf("Received Chunk: %q", string(buffer))
		p.filterCommands(&buffer, &transactionMessage)
		log.Printf("Modified Chunk: %q", string(transactionMessage[2]))
		log.Print("\n\n\n\n")

		// Convert the message representation into a bytes before sending
		modifiedChunk := bytes.Join(transactionMessage, []byte{})

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

// forwardResponsePackets forwards TCP packets from a source stream to destination steam, offloading transaction
// This function will be executed as a goroutine
func (p *TCPProxy) forwardResponsePackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {

	// Big exchange buffer
	buffer := make([]byte, 0, ConnectionResponseBufferSize)

	var responseBytes []byte

	for {
		// Read each message from the source
		n, err := sourceConn.Read(buffer[:cap(buffer)])
		if err != nil {
			if err != io.EOF {
				log.Print(fmt.Sprintf(FailedReadingResponseErrorMessage, err))
			}
			break
		}

		buffer = buffer[:n]
		//log.Printf("Response chunk: %q", string(buffer))

		// Parse the chunk to transform transaction into a single command request
		// Ref: https://pkg.go.dev/github.com/tidwall/resp#section-readme
		responseBytes, err = p.filterResponse(buffer)
		if err != nil {
			log.Print(fmt.Sprintf(FailedParsingResponseErrorMessage, err))
		}

		_, err = destConn.Write(responseBytes)
		if err != nil {
			log.Print(fmt.Sprintf(FailedWritingResponseErrorMessage, err))
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

	// Set the timeouts for the connections
	// TODO: decide the deadline policy
	frontendConn.SetDeadline(time.Time{})
	backendConn.SetDeadline(time.Time{})
	//backendConn.SetDeadline(time.Now().Add(time.Second * ConnectionTimeoutSeconds))

	// Send the request from the frontend to the backend server
	go p.forwardCommandPackets(frontendConn, backendConn, frontendClosed)

	// Send the response back to frontend contacting us.
	go p.forwardResponsePackets(backendConn, frontendConn, backendClosed)

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
