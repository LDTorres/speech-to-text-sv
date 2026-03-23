package users

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type ServiceContract interface {
	CreateUser(ctx context.Context, input CreateUserInput) (User, error)
	GetUserByID(ctx context.Context, id int64) (User, error)
	ListUsers(ctx context.Context, params ListUsersParams) ([]User, error)
}

type Handler struct {
	service ServiceContract
}

func NewHandler(service ServiceContract) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/users", func(r chi.Router) {
		r.Post("/", h.CreateUser)
		r.Get("/", h.ListUsers)
		r.Get("/{id}", h.GetUser)
	})
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var request CreateUserRequest

	if err := decodeJSON(r, &request); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error(), "invalid_request")
		return
	}

	user, err := h.service.CreateUser(r.Context(), CreateUserInput{
		Name:  request.Name,
		Email: request.Email,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, newSuccessEnvelope(newUserResponse(user)))
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	idValue := chi.URLParam(r, "id")

	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "id must be a valid integer", "invalid_request")
		return
	}

	user, err := h.service.GetUserByID(r.Context(), id)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, newSuccessEnvelope(newUserResponse(user)))
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit := DefaultListLimit
	offset := 0
	query := r.URL.Query()

	if rawLimit := query.Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "limit must be a valid integer", "invalid_request")
			return
		}

		limit = parsedLimit
	}

	if rawOffset := query.Get("offset"); rawOffset != "" {
		parsedOffset, err := strconv.Atoi(rawOffset)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "offset must be a valid integer", "invalid_request")
			return
		}

		offset = parsedOffset
	}

	users, err := h.service.ListUsers(r.Context(), ListUsersParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, newSuccessEnvelope(newUserListResponse(users, limit, offset)))
}

func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		h.writeError(w, http.StatusBadRequest, validationErr.Error(), "invalid_input")
		return
	}

	var notFoundErr *NotFoundError
	if errors.As(err, &notFoundErr) {
		h.writeError(w, http.StatusNotFound, notFoundErr.Error(), "not_found")
		return
	}

	h.writeError(w, http.StatusInternalServerError, "internal server error", "internal_error")
}

func (h *Handler) writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte(`{"data":null,"error":{"message":"internal server error","code":"internal_error"}}`)
		statusCode = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(append(body, '\n'))
}

func (h *Handler) writeError(w http.ResponseWriter, statusCode int, message string, code string) {
	h.writeJSON(w, statusCode, newErrorEnvelope(message, code))
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is required")
		}

		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			return errors.New("request body contains invalid JSON")
		}

		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return errors.New("request body contains invalid field types")
		}

		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return errors.New("request body contains unknown fields")
		}

		return errors.New("request body contains invalid JSON")
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}
