package api

import (
	"sync"
)

// ConnectionTrackKey represents a key unique key generated from connection data
// This type is used on UDP connections. Being connectionless, the requesters' personal data are
// used as unique identifier for the connection
type ConnectionTrackKey struct {
	IP   string
	Port int
}

type ConnectionTrackStorage struct {
	RedisSelectedDB int
}

// ConnectionPoolMap is a type to store connection track keys for UDP connections
type ConnectionPoolMap map[ConnectionTrackKey]ConnectionTrackStorage

// ProxyCache represents the cache storage for any proxy
type ProxyCache struct {

	// The following lock allows block accessing the cache until is unblocked,
	// to avoid too much overrides at the same time
	CacheLock sync.Mutex

	// ConnectionPool is a table storing some useful data related to the source of connection
	ConnectionPool ConnectionPoolMap
}
