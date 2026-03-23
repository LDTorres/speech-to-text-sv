package server

import (
	"net/http"
	"time"
)

type healthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

func Healthcheck(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, envelope{
		Data: healthResponse{
			Status:    "ok",
			Timestamp: time.Now().UTC(),
		},
		Error: nil,
	})
}
