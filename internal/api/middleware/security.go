package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
)

func CORS() func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}

func RateLimit() func(http.Handler) http.Handler {
	return httprate.LimitByIP(100, 1*time.Minute)
}
