package api

// Listener represents a server which is listening for the external traffic to forward it to backend servers
type Listener struct {
	Protocol string `yaml:"protocol"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
}

// Backend represents a backend server
type Backend struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// PluginConfig represents a plugin definition
type PluginConfig struct {
	Name  string `yaml:"name"`
	Path  string `yaml:"path"`
	Cache bool   `yaml:"cache,omitempty"`
}

// Pipelines represents the configuration related to plugins and how they are executed
type Pipelines struct {
	Plugins    []PluginConfig `yaml:"plugins,omitempty"`
	OnReceive  []string       `yaml:"onReceive,omitempty"`
	OnResponse []string       `yaml:"onResponse,omitempty"`
}

// Proxy represents a group composed by all the pieces needed to forward and balance the traffic for each request
type Proxy struct {
	Listener  Listener  `yaml:"listener"`
	Backend   Backend   `yaml:"backend"`
	Pipelines Pipelines `yaml:"pipelines,omitempty"`
}

// Config represents the configuration manifest to set parameters for all the listeners
type Config struct {
	ApiVersion string `yaml:"apiVersion,omitempty"`
	Kind       string `yaml:"kind,omitempty"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec Proxy `yaml:"spec"`
}
