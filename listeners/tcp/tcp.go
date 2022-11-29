package tcp

// Ref: https://scene-si.org/2020/05/29/waiting-on-goroutines/
// Ref: https://gist.github.com/jbardin/821d08cb64c01c84b81a

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/achetronic/predoxy/api"
	"github.com/tidwall/resp"
	"io"
	"net"
	"regexp"
	"strconv"
	"time"
)

const (
	ProtocolTcp                  = "tcp"
	ConnectionResponseBufferSize = 1024 * 1024 // 200KB
	ConnectionTimeoutSeconds     = 10 * 60     // 10 minutes
	DefaultDbIndex               = 0

	// Definitions of common individual commands (used to craft commands)
	CommandMulti     = "*1\r\n$5\r\nMULTI\r\n"
	CommandSelect    = "*2\r\n$6\r\nSELECT\r\n$1\r\n%d\r\n"
	CommandExec      = "*1\r\n$4\r\nEXEC\r\n"
	CommandEchoError = "*2\r\n$4\r\nECHO\r\n$8\r\nPERR#%d\r\n"

	// Definitions of common pipelined commands (used to craft commands)
	PipelinedCommandEchoError = "ECHO PERR#%d\r\n"

	// Regex patterns for individual intercepted commands
	// Ref: https://pkg.go.dev/regexp/syntax
	RegexQueryCommand = `(\*[1-9]+(\r?\n)\$7(\r?\n)(?i)(command)(\r?\n)){1}(\$[1-9]+(\r?\n)[A-Za-z]+(\r?\n))*`
	RegexQueryMulti   = `(\*[1-9]+(\r?\n)\$5(\r?\n)(?i)(multi)(\r?\n)){1}(\$[1-9]+(\r?\n)[A-Za-z]+(\r?\n))*`
	RegexQuerySelect  = `(\*[1-9]+(\r?\n)\$6(\r?\n)(?i)(select)(\r?\n)){1}(\$[1-9]+(\r?\n)(?P<database>[0-9]+)(\r?\n)){1}`

	// Regex patterns for intercepted pipelined commands
	RegexQueryPipelinedCommand = `(?m)^(?i)(command){1}([[:blank:]]*[A-Za-z]+)*`
	RegexQueryPipelinedMulti   = `(?m)^(?i)(multi){1}([[:blank:]]*[A-Za-z]+)*`
	RegexQueryPipelinedSelect  = `(?m)^(?i)(select){1}([[:blank:]]*(?P<database>[0-9]+)){1}`

	// RegexResponseEchoError is a regex pattern for intercepted responses
	// Example raw slice: $8\r\nPERR#403\r\n
	RegexResponseEchoError = `(?m)^\$8(\r?\n)(?i)(PERR#[0-9]+)(\r?\n)`

	// Error messages
	FailedWritingQueryErrorMessage            = "Error writing the query on the socket: %q"
	FailedWritingResponseErrorMessage         = "Error writing the response on the socket: %q"
	FailedReadingQueryErrorMessage            = "Error reading the query from the client socket: %q"
	FailedReadingResponseErrorMessage         = "Error reading the response from the server socket: %q"
	FailedParsingResponseErrorMessage         = "Error parsing the response: %q"
	FailedConnectionDataParsingErrorMessage   = "Error parsing the host and port from remote: %q"
	FailedClosingClientConnectionErrorMessage = "Error closing the connection on the client socket: %q"
	FailedClosingServerConnectionErrorMessage = "Error closing the connection on the server socket: %q"

	// Info messages
	ClientConnectedInfoMessage = "A new connection established from remote: %s"

	// Debug messages
	CachedDatabaseDebugMessage         = "Cached database: %d"
	ResponseBeforeFilterDebugMessage   = "Response before applying filters: %s"
	ResponseAfterFilterDebugMessage    = "Response after applying filters: %s"
	QueryBeforeFilterDebugMessage      = "Query before applying filters: %s"
	QueryAfterFilterDebugMessage       = "Query after applying filters: %s"
	ClientConnectionClosedDebugMessage = "Connection was closed on the client side"
	ServerConnectionClosedDebugMessage = "Connection was closed on the server side"
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
		//RegexQueryCommand, // TODO: Allow large responses to remove COMMAND related restrictions
	}

	// PipelinedQueryFilters groups the filters which will be applied to requested queries before executing them
	// ATTENTION: pipelined queries have not a well-defined format, so they have to be filtered after common flow
	PipelinedQueryFilters = []string{
		RegexQueryPipelinedMulti,
		//RegexQueryPipelinedCommand,
	}

	// ProxyErrorCodes represents response error codes and its messages
	// As a convention, we use HTTP error codes for ease
	// Ref: https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#client_error_responses
	// TODO craft a function to wrap the error messages automatically
	ProxyErrorCodes = map[int]string{
		403: "-ERR forbidden: command disabled on proxy",
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

	p.Logger.Debugf(CachedDatabaseDebugMessage, database)
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

	// Offload errors on the response chunk
	compiledFilter := regexp.MustCompile(RegexResponseEchoError)
	matchFilter := compiledFilter.Match(responseBytes)
	if matchFilter {
		responseBytes = compiledFilter.ReplaceAllLiteral(responseBytes, []byte(ProxyErrorCodes[403]+"\r\n"))
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
				p.Logger.Errorf(FailedReadingQueryErrorMessage, err)
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

		p.Logger.Debugf(QueryBeforeFilterDebugMessage, string(buffer))

		// Modify the request according to the filters
		//p.filterCommands(&buffer, &transactionMessage)
		//p.Logger.Debugf(QueryAfterFilterDebugMessage, string(transactionMessage[2]))

		// Convert the message representation into a bytes before sending
		//modifiedChunk := bytes.Join(transactionMessage, []byte{})
		modifiedChunk := buffer[:n]

		// Forward the command to the backend
		_, err = destConn.Write(modifiedChunk)
		if err != nil {
			p.Logger.Errorf(FailedWritingQueryErrorMessage, err)
		}

		// Clear last read content
		buffer = make([]byte, 0, ConnectionResponseBufferSize)
	}

	if err := sourceConn.Close(); err != nil {
		p.Logger.Errorf(FailedClosingClientConnectionErrorMessage, err)
	}
	sourceConnClosed <- struct{}{}

	p.Logger.Debug(ClientConnectionClosedDebugMessage)
}

// readAll buffer each message from the source connection until EOF, and return the whole message
func (p *TCPProxy) readAll(sourceConn *net.Conn) (message []byte, err error) {

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

// forwardResponsePackets forwards TCP packets from a source stream to destination steam, offloading transaction
// This function will be executed as a goroutine
func (p *TCPProxy) forwardResponsePackets(sourceConn net.Conn, destConn net.Conn, sourceConnClosed chan struct{}) {

	var message []byte
	var err error

	for {

		// Read the whole message from the source
		message, err = p.readAll(&sourceConn)
		if err != nil {
			p.Logger.Errorf(FailedReadingResponseErrorMessage, err)
			break
		}

		// Write the response to the client
		_, err = destConn.Write(message)
		if err != nil {
			p.Logger.Errorf(FailedWritingResponseErrorMessage, err)
			break
		}

		// Reset the message
		message = []byte{}
	}

	// Send the right response to the user
	if err := sourceConn.Close(); err != nil {
		p.Logger.Errorf(FailedClosingServerConnectionErrorMessage, err)
	}
	sourceConnClosed <- struct{}{}

	p.Logger.Debug(ServerConnectionClosedDebugMessage)
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

// Close TODO: implement the function
func (p *TCPProxy) Close() (err error) {
	return err
}
