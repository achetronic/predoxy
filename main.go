package main

import (
	"flag"
	"github.com/achetronic/predoxy/api"
	"github.com/achetronic/predoxy/listeners/tcp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"log"
	"math/rand"
	"time"
)

const (
	//
	DefaultConfigFilePathFlag = "sample.yaml"
	DefaultLogLevelFlag       = "info"

	//
	LogLevelFlagDescription   = "log level for the application: debug, info, error"
	ConfigFileFlagDescription = "path to the config.yaml file"

	//
	InvalidConfigErrorMessage = "Config file is not valid: %s"

	// Error messages
	ProxyLaunchMessage = "TCP proxy will be launched listening on %s:%d"
)

// LoadYAMLConfig TODO
func LoadYAMLConfig(filePath string) (config api.Config, err error) {

	// read YAML config file
	yfile, err := ioutil.ReadFile(filePath)
	if err != nil {
		return config, err
	}

	// Try to load config into the object
	err = yaml.Unmarshal(yfile, &config)
	if err != nil {
		return config, err
	}

	return config, err
}

//
func main() {

	// Feed the seed to be able to get random numbers safe across the code
	rand.Seed(time.Now().UnixNano())

	// Play with the flags
	logLevelFlag := flag.String("zap-log-level", DefaultLogLevelFlag, LogLevelFlagDescription)
	configPathFlag := flag.String("config", DefaultConfigFilePathFlag, ConfigFileFlagDescription)
	flag.Parse()

	logLevel, _ := zap.ParseAtomicLevel(*logLevelFlag)

	// Initialize the logger
	loggerConfig := zap.NewProductionConfig()
	loggerConfig.EncoderConfig.TimeKey = "timestamp"
	loggerConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)
	loggerConfig.Level.SetLevel(logLevel.Level())

	// Configure the logger
	logger, err := loggerConfig.Build()
	if err != nil {
		log.Fatal(err)
	}
	sugar := logger.Sugar()

	// Create the proxy
	mainProxy := tcp.TCPProxy{
		Config: &api.Proxy{},
		Cache:  &api.ProxyCache{}, // Create the cache for proxy
		Logger: sugar,
	}

	// Load configuration file
	config := api.Config{}
	config, err = LoadYAMLConfig(*configPathFlag)
	if err != nil {
		mainProxy.Logger.Fatal(err)
	}

	// Set the configuration inside the proxy
	mainProxy.Config = &config.Spec

	// TODO: REMOVE THIS DEBUGGING SHIT
	mainProxy.Logger.Debug(mainProxy.Config)
	mainProxy.Logger.Debug(mainProxy.Cache.ConnectionPool)

	var waitForEverything chan struct{}

	// Launch all the listeners according to their configuration
	mainProxy.Logger.Infof(ProxyLaunchMessage, mainProxy.Config.Listener.Host, mainProxy.Config.Listener.Port)
	go func() {
		err := mainProxy.Launch()
		if err != nil {

		}
	}()

	<-waitForEverything

	// Log the error if the proxy explodes
	mainProxy.Logger.Error(err)
}
