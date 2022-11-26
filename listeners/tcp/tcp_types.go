package tcp

import (
	"github.com/achetronic/predoxy/api"
	"go.uber.org/zap"
)

// TCPProxy represents a live object, created from a listener's config
type TCPProxy struct {
	Config *api.Proxy
	Cache  *api.ProxyCache
	Logger *zap.SugaredLogger
}
