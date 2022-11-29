package tcp

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/achetronic/predoxy/api"
	"github.com/tidwall/resp"
	"io"
	"log"
	"net"
	"regexp"
	"strconv"
)

const (
	DefaultDbIndex = 0

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

	// Info messages

	// Debug messages
	CachedDatabaseDebugMessage       = "Cached database: %d"
	ResponseBeforeFilterDebugMessage = "Response before applying filters: %s"
	ResponseAfterFilterDebugMessage  = "Response after applying filters: %s"
	QueryBeforeFilterDebugMessage    = "Query before applying filters: %s"
	QueryAfterFilterDebugMessage     = "Query after applying filters: %s"
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

// incomingMessagesPipeline implements the pipeline to process all incoming commands from the clients
func (p *TCPProxy) incomingMessagesPipeline(parameters *ForwardCallbackParams) ([]byte, error) {
	log.Print("Procesando la pipeline de entrada")
	return *parameters.Message, nil
}

// outgoingMessagesPipeline implements the pipeline to process all outgoing responses from the server
func (p *TCPProxy) outgoingMessagesPipeline(parameters *ForwardCallbackParams) ([]byte, error) {
	log.Print("Procesando la pipeline de salida")
	return *parameters.Message, nil
}

// doSomethingTest TODO
func (p *TCPProxy) doSomethingTest(parameters *ForwardCallbackParams) ([]byte, error) {

	var err error

	// Construct a transaction message from the wrapper
	transactionMessage := MessageTransaction
	transactionMessage[2] = *parameters.Message

	// Parse the request to lazy update cached db index
	// TODO: Improve the performance of this logic
	p.setCachedDB(parameters.SourceConnection, parameters.Message)
	dbIndex, _ := p.getCachedDB(parameters.SourceConnection)
	transactionMessage[1] = []byte(fmt.Sprintf(CommandSelect, dbIndex))

	p.Logger.Debugf(QueryBeforeFilterDebugMessage, string(*parameters.Message))

	// Modify the request according to the filters
	p.filterCommands(parameters.Message, &transactionMessage)
	p.Logger.Debugf(QueryAfterFilterDebugMessage, string(transactionMessage[2]))

	// Convert the message representation into a bytes before sending
	modifiedMessage := bytes.Join(transactionMessage, []byte{})
	log.Print(string(modifiedMessage))

	return modifiedMessage, err
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
