package api

// Plugins are a core feature for this proxy. Due to that, they are defined in the
// 'api' package, which is the package intended for important base definitions

import (
	"github.com/allegro/bigcache/v3"
	"net"
)

// PluginParams represents the parameters passed to all functions for Plugin interface
type PluginParams struct {
	SourceConnection *net.Conn
	DestConnection   *net.Conn
	Message          *[]byte
	LocalCache       *bigcache.BigCache
}

// Plugin represents the interface that must be implemented by any plugin
type Plugin interface {

	// OnReceive is a function to process a message on the requests pipeline
	OnReceive(*PluginParams) error

	// OnResponse is a function to process a message on the responses pipeline
	OnResponse(*PluginParams) error
}
