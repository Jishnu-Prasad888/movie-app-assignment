// Package middleware contains generic HTTP middleware (logging, panic recovery, CSRF header
// check, security headers).
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"yozigo/internal/apperr"
	"yozigo/internal/httpx"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("panic", "path", r.URL.Path, "panic", v)
				httpx.JSON(w, http.StatusInternalServerError, map[string]string{
					"error": "Something went wrong. Please try again.", "code": "INTERNAL",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CSRFHeader is CSRF defence layer 2 (layer 1 is the SameSite=Lax cookie). Every
// state-changing request must carry "X-Requested-With: XMLHttpRequest". HTML forms on other
// sites cannot set custom headers, and cross-origin fetch() with a custom header needs a CORS
// preflight, which this API never approves.
func CSRFHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				httpx.Error(w, r, apperr.New(apperr.Forbidden, "CSRF_HEADER_MISSING",
					"Missing X-Requested-With header."))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
