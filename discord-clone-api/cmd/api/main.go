package main

import (
	"log/slog"
	"net/http"
	"os"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	log.Info("api server starting", "addr", ":8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}