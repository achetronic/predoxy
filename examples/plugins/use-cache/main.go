package main

import (
	"github.com/achetronic/predoxy/api"
	"log"
)

type customPlugin string

// OnReceive TODO
func (p *customPlugin) OnReceive(parameters *api.PluginParams) error {

	log.Print("Store a value on your local cache")

	err := (*parameters).LocalCache.Set("foo", []byte("FOO"))
	if err != nil {
		log.Print("Error storing the value")
	}

	return nil
}

// OnResponse TODO
func (p *customPlugin) OnResponse(parameters *api.PluginParams) error {

	log.Print("Get a value from your local cache")

	value, err := (*parameters).LocalCache.Get("foo")
	if err != nil {
		log.Print("Error getting the value")
	}

	log.Printf("The value is: %q", string(value))

	return nil
}

var Plugin customPlugin
