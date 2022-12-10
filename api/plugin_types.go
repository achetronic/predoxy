package api

import "net"

// PluginParams represents the parameters passed to all functions for Plugin interface
type PluginParams struct {
	SourceConnection *net.Conn
	DestConnection   *net.Conn
	Message          *[]byte
}

// Plugin represents the interface that must be implemented by any plugin
type Plugin interface {

	// OnReceive is a function to process a message on the requests pipeline
	OnReceive(*PluginParams) error

	// OnResponse is a function to process a message on the responses pipeline
	OnResponse(*PluginParams) error
}
