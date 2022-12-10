package main

import (
	"github.com/achetronic/predoxy/api"
	"log"
)

type customPlugin string

// OnReceive TODO
func (p *customPlugin) OnReceive(parameters *api.PluginParams) error {

	log.Print("Basically, I'm not doing anything on receive")
	log.Print(string(*(*parameters).Message))
	return nil
}

// OnResponse TODO
func (p *customPlugin) OnResponse(parameters *api.PluginParams) error {

	log.Print("Basically, I'm not doing anything on response")
	log.Print(string(*(*parameters).Message))
	return nil
}

var Plugin customPlugin
