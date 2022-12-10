package api

import (
	"net"
)

// PipelineCallbackParams represents the parameters passed to the callback on forwardPackets function
type PipelineCallbackParams struct {
	ProxyCache       *ProxyCache
	SourceConnection *net.Conn
	DestConnection   *net.Conn
	Message          *[]byte
}

// PipelineCallback represents a function to process a message before writing it to a TCP connection
type PipelineCallback func(*PipelineCallbackParams) ([]byte, error)
