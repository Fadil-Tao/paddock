package transport

import (
	"net/http"

	"github.com/Fadil-Tao/paddock/internal/transport/middleware"
)

type SandboxHandler interface {
	Create(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	GetById(w http.ResponseWriter, r *http.Request)
	Remove(w http.ResponseWriter, r *http.Request)
	UpdateSandbox(w http.ResponseWriter, r *http.Request)
	Exec(w http.ResponseWriter, r *http.Request)
	Logs(w http.ResponseWriter, r *http.Request)
}

type HttpRouter struct {
	sandboxHandler SandboxHandler
}

func NewHttpRouter(sandboxHandler SandboxHandler) *HttpRouter {
	return &HttpRouter{
		sandboxHandler,
	}
}

func (h HttpRouter) Route() *http.ServeMux {
	mux := http.NewServeMux()
	sandboxHandler := h.sandboxHandler

	mux.HandleFunc("GET /sandboxes", sandboxHandler.Get)
	mux.HandleFunc("GET /sandboxes/{id}", sandboxHandler.GetById)
	mux.HandleFunc("POST /sandboxes", sandboxHandler.Create)
	mux.HandleFunc("DELETE /sandboxes/{id}", sandboxHandler.Remove)
	mux.HandleFunc("PATCH /sandboxes/{id}", sandboxHandler.UpdateSandbox)
	mux.HandleFunc("POST /sandboxes/{id}/execs", sandboxHandler.Exec)
	mux.HandleFunc("GET /sandboxes/{id}/logs", sandboxHandler.Logs)

	api :=  http.NewServeMux()
	api.Handle("/api/", http.StripPrefix("/api", middleware.ApiKeyMiddleware(mux)))
	return api
}
