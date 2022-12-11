package api

import (
	"github.com/allegro/bigcache/v3"
	"sync"
)

// PluginCache represents TODO
type PluginCache struct {

	// Pool represents a map where the key is the name of the plugin,
	// and the value is a pointer to the plugin loaded on memory
	Pool map[string]*Plugin

	// LocalCachePool represents a map where the key is the name of the plugin,
	// and the value is a pointer to an instance of BigCache.
	// This is only created when 'cache: true' on configuration
	LocalCachePool map[string]*bigcache.BigCache

	ExecutionOrder struct {
		// OnReceive stores the execution order of the plugins for incoming messages
		OnReceive []string

		// OnResponse stores the execution order of the plugins for outgoing messages
		OnResponse []string
	}
}

// ProxyCache represents the cache storage for any proxy
type ProxyCache struct {

	// The following lock allows block accessing the cache until is unblocked,
	// to avoid too much overrides at the same time
	CacheLock sync.Mutex

	// PluginCache store the plugins loaded on memory and its execution order
	// from the config
	PluginCache PluginCache
}
