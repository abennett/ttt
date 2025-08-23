package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func health(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("ok"))
}

func NewMux(server *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.DefaultLogger)
	r.Get("/{roomName}", server.ServeHTTP)
	r.Get("/health", health)
	return r
}
