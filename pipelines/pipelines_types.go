package pipelines

import (
	"github.com/achetronic/predoxy/api"
	"net"
)

// ForwardCallbackParams represents the parameters passed to the callback on forwardPackets function
type ForwardCallbackParams struct {
	SourceConnection *net.Conn
	DestConnection   *net.Conn
	ProxyCache       *api.ProxyCache
	Message          *[]byte
}

// ForwardCallback represents a function to process a message before writing it to a TCP connection
type ForwardCallback func(*ForwardCallbackParams) ([]byte, error)
