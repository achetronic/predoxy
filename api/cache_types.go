package api

import (
	"plugin"
	"sync"
)

// ProxyCache represents the cache storage for any proxy
type ProxyCache struct {

	// The following lock allows block accessing the cache until is unblocked,
	// to avoid too much overrides at the same time
	CacheLock sync.Mutex

	// PluginPool
	PluginPool map[string]*plugin.Symbol
}
