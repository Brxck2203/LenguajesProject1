package httptransport

import (
	"encoding/json"
	"log"
	"net/http"
)

func write_json(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("failed to encode HTTP JSON response: %v", err)
	}
}

func write_error(writer http.ResponseWriter, status int, message string) {
	write_json(writer, status, struct {
		Error string `json:"error"`
	}{Error: message})
}
