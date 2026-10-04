package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Estructuras para deserializar el JSON de Scala
type MMAMetrics struct {
	StrikeAccuracyPercentage float64 `json:"strikeAccuracyPercentage"`
}

type FreeFireMetrics struct {
	KDRatio     float64 `json:"kdRatio"`
	TotalDamage float64 `json:"totalDamage"`
}

type AnalyticsResponse struct {
	PlayerID string          `json:"playerId"`
	MMA      MMAMetrics      `json:"mma"`
	FreeFire FreeFireMetrics `json:"freeFire"`
}

func main() {
	fmt.Println("Servidor Go listo en el puerto 8081...")

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok", "service":"go-proxy"}`))
	})

	http.HandleFunc("/scala-metrics", func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Get("http://scala-analytics:8080/analytics")
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Conexión fallida: %v"}`, err), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, `{"error": "Error al leer respuesta"}`, http.StatusInternalServerError)
			return
		}

		// Parsear el JSON recibido desde Scala
		var analytics AnalyticsResponse
		if err := json.Unmarshal(body, &analytics); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "JSON invalido: %v"}`, err), http.StatusBadRequest)
			return
		}

		// Responder desde Go con el objeto estructurado
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(analytics)
	})

	http.ListenAndServe(":8081", nil)
}
