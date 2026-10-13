// Command payments-api is a small charges service: create a charge, read it
// back, retry safely. It keeps everything in memory.
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// newHandler is the whole HTTP surface, separate from main so tests can drive
// it without opening a port.
func newHandler(l *Ledger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("POST /v1/charges", func(w http.ResponseWriter, r *http.Request) {
		var req ChargeRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "the body is not a charge: "+err.Error())
			return
		}
		c, replay, err := l.Create(r.Header.Get("Idempotency-Key"), req)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if strings.Contains(err.Error(), "idempotency key") {
				status = http.StatusConflict
			}
			writeError(w, status, err.Error())
			return
		}
		status := http.StatusCreated
		if replay {
			// The same answer as the first time, and a header saying so, so a
			// client can tell a retry from a new charge.
			w.Header().Set("Idempotent-Replay", "true")
			status = http.StatusOK
		}
		writeJSON(w, status, c)
	})

	mux.HandleFunc("GET /v1/charges/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := l.Get(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "no such charge")
			return
		}
		writeJSON(w, http.StatusOK, c)
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           newHandler(NewLedger(time.Now)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("payments-api listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
