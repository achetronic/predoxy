package tcp

import (
	"github.com/achetronic/redis-proxy/api"
	"io"
)

// TCPProxy represents a live object, created from a listener's config
type TCPProxy struct {
	Config *api.Proxy
	Cache  *api.ProxyCache
	Logger *io.Writer
}
