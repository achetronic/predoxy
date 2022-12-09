package pipeline

// Plugin represents the group of functions that must be implemented by any plugin
type Plugin interface {

	// OnReceive is a function to process a message on the incoming pipeline
	OnReceive(*ForwardCallbackParams) error

	// OnResponse is a function to process a message on the outgoing pipeline
	OnResponse(*ForwardCallbackParams) error
}
