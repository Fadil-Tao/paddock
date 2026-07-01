package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"
)

func ApiKeyMiddleware(next http.Handler) http.Handler {
    apiKey := os.Getenv("PADDOCK_API_KEY")
    if apiKey == "" {
        log.Fatal().Msg("PADDOCK_API_KEY not set")
    }
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        incoming := r.Header.Get("X-API-Key")
		
        if subtle.ConstantTimeCompare([]byte(incoming), []byte(apiKey)) != 1{
            log.Warn().Msg("request failed api key authentication")
            w.Header().Set("Content-Type", "application/json; charset=utf-8")
            w.WriteHeader(http.StatusUnauthorized)
            json.NewEncoder(w).Encode(map[string]string{"message": "invalid api key"})
            return
        }
        next.ServeHTTP(w, r)
    })
}