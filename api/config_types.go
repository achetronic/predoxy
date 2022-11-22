package api

// Backend represents a backend server
type Backend struct {
	Host string `yaml:"host,omitempty"`
	Port int    `yaml:"port,omitempty"`
}

// Listener represents a server which is listening for the external traffic to forward it to backend servers
type Listener struct {
	Protocol string `yaml:"protocol,omitempty"`
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
}

// Proxy represents a group composed by all the pieces needed to forward and balance the traffic for each request
type Proxy struct {
	Listener Listener `yaml:"listener,omitempty"`
	Backend  Backend  `yaml:"backend,omitempty"`
}

// Config represents the configuration manifest to set parameters for all the listeners
type Config struct {
	ApiVersion string `yaml:"apiVersion,omitempty"`
	Kind       string `yaml:"kind,omitempty"`
	Metadata   struct {
		Name string `yaml:"name,omitempty"`
	} `yaml:"metadata,omitempty"`
	Spec Proxy `yaml:"spec,omitempty"`
}
