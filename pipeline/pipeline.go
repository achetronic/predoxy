package pipeline

import (
	"github.com/achetronic/predoxy/api"
)

// ProcessIncomingMessages implements the pipeline to process all incoming commands from the clients
func ProcessIncomingMessages(parameters *api.PipelineCallbackParams) ([]byte, error) {

	// 0. Generate the params to execute all the Plugin's OnReceive functions
	pluginParams := api.PluginParams{
		SourceConnection: parameters.SourceConnection,
		DestConnection:   parameters.DestConnection,
		Message:          parameters.Message,
	}

	// 1. loop over the OnReceive list of plugins
	// For each plugin, execute the function introducing a pointer to the params
	for _, pluginName := range parameters.ProxyCache.PluginCache.ExecutionOrder.OnReceive {
		(*parameters.ProxyCache.PluginCache.Pool[pluginName]).OnReceive(&pluginParams)
	}

	return *parameters.Message, nil
}

// ProcessOutgoingMessages implements the pipeline to process all outgoing responses from the server
func ProcessOutgoingMessages(parameters *api.PipelineCallbackParams) ([]byte, error) {

	// 0. Generate the params to execute all the Plugin's OnReceive functions
	pluginParams := api.PluginParams{
		SourceConnection: parameters.SourceConnection,
		DestConnection:   parameters.DestConnection,
		Message:          parameters.Message,
	}

	// 1. loop over the OnReceive list of plugins
	// For each plugin, execute the function introducing a pointer to the params
	for _, pluginName := range parameters.ProxyCache.PluginCache.ExecutionOrder.OnReceive {
		(*parameters.ProxyCache.PluginCache.Pool[pluginName]).OnResponse(&pluginParams)
	}

	return *parameters.Message, nil
}
