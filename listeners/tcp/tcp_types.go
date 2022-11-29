package tcp

import (
	"github.com/achetronic/predoxy/api"
	"go.uber.org/zap"
	"net"
)

// TCPProxy represents a live object, created from a listener's config
type TCPProxy struct {
	Config *api.Proxy
	Cache  *api.ProxyCache
	Logger *zap.SugaredLogger
}

// ForwardCallbackParams represents the parameters passed to the callback on forwardPackets function
type ForwardCallbackParams struct {
	SourceConnection *net.Conn
	DestConnection   *net.Conn
	Cache            *api.ProxyCache
	Message          *[]byte
}

// ForwardCallback represents a function to process a message before writing it to a TCP connection
type ForwardCallback func(*ForwardCallbackParams) ([]byte, error)
