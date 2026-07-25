package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
)

func CORS() func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins: []string{"https://*", "http://*"},
		// Must cover every verb the router registers, or the browser blocks the request
		// at the preflight - see TestCORS_AllowsEveryVerbTheRouterRegisters.
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}

func RateLimit() func(http.Handler) http.Handler {
	return httprate.LimitByIP(100, 1*time.Minute)
}
