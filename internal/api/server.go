// Package api serves the governor's status and settings over HTTP. The
// Docker Desktop UI reaches it through the extension's unix socket.
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/QuantumNovice/docker-gpu-governor/internal/governor"
)

// Governor is the part of the governor the API exposes.
type Governor interface {
	Status() governor.Status
	Config() governor.Config
	SetConfig(governor.Config) error
	ResumeAll(ctx context.Context) error
}

// Handler returns the API routes.
//
//	GET  /status   latest readings, containers and activity log
//	GET  /config   current settings
//	POST /config   replace settings (the full object)
//	POST /resume   resume everything the governor paused
//	GET  /healthz  liveness check
func Handler(g Governor) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, g.Status())
	})
	mux.HandleFunc("GET /config", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, g.Config())
	})
	mux.HandleFunc("POST /config", func(w http.ResponseWriter, r *http.Request) {
		var cfg governor.Config
		dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, "invalid settings: "+err.Error())
			return
		}
		if err := g.SetConfig(cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, g.Config())
	})
	mux.HandleFunc("POST /resume", func(w http.ResponseWriter, r *http.Request) {
		if err := g.ResumeAll(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, g.Status())
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"message": msg})
}
