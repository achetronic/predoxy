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
	Message          *[]byte
}

//
type ForwardCallback func(*ForwardCallbackParams) ([]byte, error)
