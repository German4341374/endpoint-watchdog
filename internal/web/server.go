// Package web exposes the health API, status API, and server-rendered status page.
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/German4341374/endpoint-watchdog/internal/monitor"
)

//go:embed templates/status.html
var templateFiles embed.FS

// Server owns the HTTP handlers without starting a network listener.
type Server struct {
	title    string
	store    *monitor.Store
	logger   *slog.Logger
	template *template.Template
}

type pageData struct {
	Title       string
	State       monitor.State
	GeneratedAt time.Time
	Endpoints   []monitor.EndpointStatus
	Changes     []changeView
}

type changeView struct {
	Endpoint string
	From     monitor.State
	To       monitor.State
	At       time.Time
}

// New creates a status server and parses its embedded template.
func New(title string, store *monitor.Store, logger *slog.Logger) (*Server, error) {
	functions := template.FuncMap{
		"stateClass": func(state monitor.State) string { return strings.ToLower(string(state)) },
		"milliseconds": func(value float64, checks int) string {
			if checks == 0 {
				return "—"
			}
			return fmt.Sprintf("%.1f ms", value)
		},
		"timestamp": func(value *time.Time) string {
			if value == nil {
				return "Waiting for first check"
			}
			return value.UTC().Format("2006-01-02 15:04:05 UTC")
		},
		"changeTime": func(value time.Time) string {
			return value.UTC().Format("15:04:05 UTC")
		},
		"uptime": func(value float64, checks int) string {
			if checks == 0 {
				return "—"
			}
			return fmt.Sprintf("%.2f%%", value)
		},
	}
	parsed, err := template.New("status.html").Funcs(functions).ParseFS(
		templateFiles,
		"templates/status.html",
	)
	if err != nil {
		return nil, fmt.Errorf("parse status template: %w", err)
	}
	return &Server{title: title, store: store, logger: logger, template: parsed}, nil
}

// Handler returns the complete application handler with security and logging middleware.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /api/status", server.status)
	mux.HandleFunc("GET /api/status/{name}", server.endpointStatus)
	mux.HandleFunc("GET /", server.index)
	return server.securityHeaders(server.requestLogger(mux))
}

func (server *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "endpoint-watchdog",
		"time":    time.Now().UTC(),
	})
}

func (server *Server) status(writer http.ResponseWriter, _ *http.Request) {
	statuses := server.store.All(false)
	writeJSON(writer, http.StatusOK, monitor.Summary{
		Service:     server.title,
		State:       monitor.OverallState(statuses),
		GeneratedAt: time.Now().UTC(),
		Endpoints:   statuses,
	})
}

func (server *Server) endpointStatus(writer http.ResponseWriter, request *http.Request) {
	name := request.PathValue("name")
	status, found := server.store.Get(name, true)
	if !found {
		writeJSON(writer, http.StatusNotFound, map[string]any{
			"error": map[string]string{
				"code":    "ENDPOINT_NOT_FOUND",
				"message": "The requested endpoint is not configured",
			},
		})
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (server *Server) index(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	statuses := server.store.All(false)
	data := pageData{
		Title:       server.title,
		State:       monitor.OverallState(statuses),
		GeneratedAt: time.Now().UTC(),
		Endpoints:   statuses,
		Changes:     collectChanges(statuses),
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	if err := server.template.ExecuteTemplate(writer, "status.html", data); err != nil {
		server.logger.Error("render status page", "error", err)
	}
}

func collectChanges(statuses []monitor.EndpointStatus) []changeView {
	changes := make([]changeView, 0)
	for _, status := range statuses {
		for _, change := range status.StateChanges {
			changes = append(changes, changeView{
				Endpoint: status.Name,
				From:     change.From,
				To:       change.To,
				At:       change.At,
			})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].At.After(changes[j].At) })
	if len(changes) > 12 {
		changes = changes[:12]
	}
	return changes
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func (server *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(writer, request)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *responseRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (server *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		server.logger.Info(
			"http request",
			"method", request.Method,
			"path", request.URL.Path,
			"status", recorder.status,
			"duration_ms", float64(time.Since(started).Microseconds())/1000,
		)
	})
}
