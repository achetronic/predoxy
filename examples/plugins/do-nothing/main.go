package main

import (
	"github.com/achetronic/predoxy/pipeline"
	"log"
)

// OnReceive TODO
func OnReceive(parameters *pipeline.ForwardCallbackParams) error {

	log.Print((*parameters).Message)
	return nil
}

// OnResponse TODO
func OnResponse(parameters *pipeline.ForwardCallbackParams) error {

	log.Print((*parameters).Message)
	return nil
}

func main() {}
