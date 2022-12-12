package pipeline

import (
	"github.com/achetronic/predoxy/api"
)

// ProcessIncomingMessages implements the pipeline to process all incoming commands from the clients
func ProcessIncomingMessages(parameters *CallbackParams) ([]byte, error) {

	// 0. Generate the params to execute all the Plugin's OnReceive functions
	pluginParams := api.PluginParams{
		SourceConnection: parameters.SourceConnection,
		DestConnection:   parameters.DestConnection,
		Message:          parameters.Message,
	}

	localCachePool := parameters.ProxyCache.PluginCache.LocalCachePool

	// 1. loop over the OnReceive list of plugins
	for _, pluginName := range parameters.ProxyCache.PluginCache.ExecutionOrder.OnReceive {

		// 1.1 Point to LocalCache when exists in LocalCachePool
		localCache, ok := localCachePool[pluginName]
		if ok {
			pluginParams.LocalCache = localCache
		}

		// 1.2 For each plugin, execute the function introducing a pointer to the params
		(*parameters.ProxyCache.PluginCache.Pool[pluginName]).OnReceive(&pluginParams)
	}

	return *parameters.Message, nil
}

// ProcessOutgoingMessages implements the pipeline to process all outgoing responses from the server
func ProcessOutgoingMessages(parameters *CallbackParams) ([]byte, error) {

	// 0. Generate the params to execute all the Plugin's OnReceive functions
	pluginParams := api.PluginParams{
		SourceConnection: parameters.SourceConnection,
		DestConnection:   parameters.DestConnection,
		Message:          parameters.Message,
	}

	localCachePool := parameters.ProxyCache.PluginCache.LocalCachePool

	// 1. loop over the OnReceive list of plugins
	for _, pluginName := range parameters.ProxyCache.PluginCache.ExecutionOrder.OnReceive {

		// 1.1 Point to LocalCache when exists in LocalCachePool
		localCache, ok := localCachePool[pluginName]
		if ok {
			pluginParams.LocalCache = localCache
		}

		// 1.2 For each plugin, execute the function introducing a pointer to the params
		(*parameters.ProxyCache.PluginCache.Pool[pluginName]).OnResponse(&pluginParams)
	}

	return *parameters.Message, nil
}
