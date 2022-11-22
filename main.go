package main

import (
	"fmt"
	"github.com/achetronic/redis-proxy/api"
	"github.com/achetronic/redis-proxy/listeners/tcp"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"log"
	"math/rand"
	"time"
)

const (
	ConfigFile = "sample.yaml"

	//
	InvalidConfigErrorMessage = "Config file is not valid: %s"
)

// LoadYAMLConfig TODO
func LoadYAMLConfig(filePath string) (config api.Config, err error) {

	// read YAML config file
	yfile, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatal(err)
	}

	// Try to load config into the object
	err = yaml.Unmarshal(yfile, &config)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("--- m:\n%v\n\n", config.Metadata.Name)

	return
}

//
func main() {

	// Feed the seed to be able to get random numbers safe across the code
	rand.Seed(time.Now().UnixNano())

	// Load configuration file
	var config api.Config
	config, err := LoadYAMLConfig(ConfigFile)

	// Create the cache for proxy
	pCache := api.ProxyCache{}

	mainProxy := tcp.TCPProxy{
		Config: &config.Spec,
		Cache:  &pCache,
	}

	log.Print(*mainProxy.Config)
	log.Print(*mainProxy.Cache)

	var waitForEverything chan struct{}

	// Launch all the listeners according to their configuration
	log.Printf("Levantamos un proxy TCP, escuchando en %s:%d", mainProxy.Config.Listener.Host, mainProxy.Config.Listener.Port)
	go func() {
		err := mainProxy.Launch()
		if err != nil {

		}
	}()

	<-waitForEverything

	log.Print(err)
}
