package pipeline

import (
	"github.com/achetronic/predoxy/api"
	"net"
)

// CallbackParams represents the parameters passed to the callback on forwardPackets function
type CallbackParams struct {
	ProxyCache       *api.ProxyCache
	SourceConnection *net.Conn
	DestConnection   *net.Conn
	Message          *[]byte
}

// Callback represents a function to process a message before writing it to a TCP connection
type Callback func(*CallbackParams) ([]byte, error)
