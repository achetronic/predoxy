package pipelines

import (
	"log"
)

// IncomingMessagesPipeline implements the pipeline to process all incoming commands from the clients
func IncomingMessagesPipeline(parameters *ForwardCallbackParams) ([]byte, error) {
	log.Print("Procesando la pipeline de entrada")
	return *parameters.Message, nil
}

// OutgoingMessagesPipeline implements the pipeline to process all outgoing responses from the server
func OutgoingMessagesPipeline(parameters *ForwardCallbackParams) ([]byte, error) {
	log.Print("Procesando la pipeline de salida")
	return *parameters.Message, nil
}
