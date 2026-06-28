package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Fadil-Tao/paddock/internal/model"
)

type DockerRunner interface {
	Create(ctx context.Context) (*model.Sandbox, error)
	List(ctx context.Context) (*[]model.Sandbox, error)
	Get(ctx context.Context, id string) (*model.Sandbox, error)
	Remove(ctx context.Context, id string) error
	Stop(ctx context.Context, id string) error
	Start(ctx context.Context, id string) error
	Exec(ctx context.Context, id string, cmd []string) (string, string, int, error)
	Log(ctx context.Context, id string, tail int) (string, error)
}

type SandboxHandler struct {
	dockerRunner DockerRunner
}

func NewSandboxHandler(dockerRunner DockerRunner) *SandboxHandler {
	return &SandboxHandler{dockerRunner}
}

func (h *SandboxHandler) Create(w http.ResponseWriter, r *http.Request) {
	sandbox, err := h.dockerRunner.Create(r.Context())
	if err != nil {
		JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	JSONSuccess(w, "sandbox created", sandbox, http.StatusCreated)
}

func (h *SandboxHandler) Get(w http.ResponseWriter, r *http.Request) {
	sandboxes, err := h.dockerRunner.List(r.Context())
	if err != nil {
		JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	JSONSuccess(w, "sandboxes retrieved", sandboxes, http.StatusOK)
}

func (h *SandboxHandler) GetById(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	sandbox, err := h.dockerRunner.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrSandboxNotFound) {
			JSONError(w, "sandbox not found", http.StatusNotFound)
			return
		}
		JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	JSONSuccess(w, "sandbox retrieved", sandbox, http.StatusOK)
}

func (h *SandboxHandler) Remove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := h.dockerRunner.Remove(r.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrSandboxNotFound) {
			JSONError(w, "sandbox not found", http.StatusNotFound)
			return
		}
		JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	JSONSuccess(w, "sandbox removed", nil, http.StatusOK)
}

func (h *SandboxHandler) UpdateSandbox(w http.ResponseWriter, r *http.Request) {
	var body struct {
		State string `json:"state"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if body.State == "" {
		JSONError(w, "state is required", http.StatusBadRequest)
		return
	}

	id := r.PathValue("id")

	var err error
	var message string

	switch body.State {
	case "running":
		err = h.dockerRunner.Start(r.Context(), id)
		message = "sandbox started"

	case "stopped":
		err = h.dockerRunner.Stop(r.Context(), id)
		message = "sandbox stopped"

	default:
		JSONError(w, "invalid state", http.StatusBadRequest)
		return
	}

	if err != nil {
		if errors.Is(err, model.ErrSandboxNotFound) {
			JSONError(w, "sandbox not found", http.StatusNotFound)
			return
		}

		JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, message, nil, http.StatusOK)
}

func (h *SandboxHandler) Exec(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Cmd []string `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(body.Cmd) == 0 {
		JSONError(w, "cmd is required", http.StatusBadRequest)
		return
	}

	stdout, stderr, exitCode, err := h.dockerRunner.Exec(r.Context(), id, body.Cmd)
	if err != nil {
		if errors.Is(err, model.ErrSandboxNotFound) {
			JSONError(w, "sandbox not found", http.StatusNotFound)
			return
		}
		JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, "exec completed", map[string]any{
		"stdout":   stdout,
		"stderr":   stderr,
		"exitCode": exitCode,
	}, http.StatusOK)
}

func (h *SandboxHandler) Logs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	tail := 0
	if t := r.URL.Query().Get("tail"); t != "" {
		parsed, err := strconv.Atoi(t)
		if err != nil {
			JSONError(w, "invalid tail parameter", http.StatusBadRequest)
			return
		}
		tail = parsed
	}

	logs, err := h.dockerRunner.Log(r.Context(), id, tail)
	if err != nil {
		if errors.Is(err, model.ErrSandboxNotFound) {
			JSONError(w, "sandbox not found", http.StatusNotFound)
			return
		}
		JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, "logs retrieved", map[string]string{"logs": logs}, http.StatusOK)
}
