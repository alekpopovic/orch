package api

import (
	"net/http"
	"strings"
	"time"
)

type Deprecation struct {
	Sunset time.Time
	Link   string
}

func apiVersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/v1" {
			w.Header().Set("API-Version", "v1")
		}
		next.ServeHTTP(w, r)
	})
}

func deprecated(handler http.Handler, metadata Deprecation) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Deprecation", "true")
		if !metadata.Sunset.IsZero() {
			w.Header().Set("Sunset", metadata.Sunset.UTC().Format(http.TimeFormat))
		}
		if strings.TrimSpace(metadata.Link) != "" {
			w.Header().Add("Link", "<"+strings.TrimSpace(metadata.Link)+">; rel=\"deprecation\"")
		}
		handler.ServeHTTP(w, r)
	})
}
