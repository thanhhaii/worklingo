// Package http wires the chi router and middleware.
package http

import (
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/worklingo/backend/internal/auth"
	"github.com/worklingo/backend/internal/http/handlers"
	"github.com/worklingo/backend/internal/ws"
)

type Deps struct {
	H          *handlers.Handlers
	Hub        *ws.Hub
	JWT        *auth.JWT
	CORSOrigin string
	Log        *slog.Logger
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(logging(d.Log))
	r.Use(recoverer(d.Log))
	r.Use(cors(d.CORSOrigin))

	r.Get("/ws", wsHandler(d))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", d.H.Health)
		r.Post("/auth/register", d.H.Register)
		r.Post("/auth/login", d.H.Login)

		r.Group(func(r chi.Router) {
			r.Use(requireAuth(d.JWT))
			r.Get("/me", d.H.Me)
			r.Get("/personas", d.H.Personas)
			r.Post("/sessions", d.H.CreateSession)
			r.Get("/sessions", d.H.ListSessions)
			r.Get("/sessions/active", d.H.ActiveSession)
			r.Get("/sessions/{id}", d.H.GetSession)
			r.Post("/sessions/{id}/end", d.H.EndSession)
			r.Get("/sessions/{id}/evaluation", d.H.Evaluation)
			r.Get("/sessions/{id}/channels/{channelId}/messages", d.H.ListMessages)
			r.Post("/sessions/{id}/channels/{channelId}/messages", d.H.PostMessage)
		})
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		handlers.Error(w, http.StatusNotFound, "not_found", "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		handlers.Error(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})
	return r
}

func wsHandler(d Deps) http.HandlerFunc {
	var patterns []string
	if u, err := url.Parse(d.CORSOrigin); err == nil && u.Host != "" {
		patterns = append(patterns, u.Host)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.URL.Query().Get("token")
		if tok == "" {
			tok = bearer(r)
		}
		uid, err := d.JWT.Parse(tok)
		if err != nil {
			handlers.Error(w, http.StatusUnauthorized, "unauthorized", "invalid or missing token")
			return
		}
		d.Hub.Serve(w, r, uid, patterns)
	}
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func requireAuth(j *auth.JWT) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, err := j.Parse(bearer(r))
			if err != nil {
				handlers.Error(w, http.StatusUnauthorized, "unauthorized", "invalid or missing token")
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithUserID(r.Context(), uid)))
		})
	}
}

func cors(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if o := r.Header.Get("Origin"); o != "" && (o == origin || origin == "*") {
				w.Header().Set("Access-Control-Allow-Origin", o)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			if r.URL.Path == "/ws" {
				return // long-lived; logged by hub
			}
			log.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(),
				"dur", time.Since(start).Round(time.Millisecond).String(), "reqId", middleware.GetReqID(r.Context()))
		})
	}
}

func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler {
						panic(rec)
					}
					log.Error("panic", "panic", rec, "stack", string(debug.Stack()))
					handlers.Error(w, http.StatusInternalServerError, "internal", "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
