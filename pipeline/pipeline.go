package pipeline

import (
	"log"
)

// ProcessIncomingMessages implements the pipeline to process all incoming commands from the clients
func ProcessIncomingMessages(parameters *ForwardCallbackParams) ([]byte, error) {
	log.Print("Procesando la pipeline de entrada")

	//algo.OnReceive(parameters *ForwardCallbackParams) error
	//algo.OnResponse(parameters *ForwardCallbackParams) error

	Test()

	log.Print("Hacemos rollos de nuevo y palante")

	return *parameters.Message, nil
}

// ProcessOutgoingMessages implements the pipeline to process all outgoing responses from the server
func ProcessOutgoingMessages(parameters *ForwardCallbackParams) ([]byte, error) {
	log.Print("Procesando la pipeline de salida")
	return *parameters.Message, nil
}
