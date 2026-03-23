package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
)

type RouteRegistrar interface {
	RegisterRoutes(r chi.Router)
}

func NewRouter(log *zap.Logger, requestTimeout time.Duration, registrars ...RouteRegistrar) http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(RequestLogger(log))
	router.Use(Recoverer(log))
	router.Use(Timeout(requestTimeout))

	router.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", Healthcheck)

		for _, registrar := range registrars {
			registrar.RegisterRoutes(r)
		}
	})

	return otelhttp.NewHandler(router, "http.server")
}
