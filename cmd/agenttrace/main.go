// Package main is the entry point for the AgentTrace server.
// It starts an HTTP server, serving the embedded React SPA and API.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/iFurySt/agenttrace/internal/api"
	"github.com/iFurySt/agenttrace/internal/otel"
	"github.com/iFurySt/agenttrace/internal/storage"
	"github.com/iFurySt/agenttrace/internal/web"
)

var (
	port      = flag.Int("port", 8080, "HTTP server port")
	dbPath    = flag.String("db", defaultDBPath(), "SQLite database path (use :memory: for in-memory)")
	dev       = flag.Bool("dev", false, "Dev mode: proxy frontend to localhost:5173")
	otlpExport = flag.String("otlp", "", "OTLP/HTTP export endpoint (e.g. http://localhost:4318)")

	// set via ldflags at build time
	version = "dev"
)

func defaultDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agenttrace", "data.db")
}

func main() {
	flag.Parse()

	log.SetFlags(0) // clean output, no timestamps
	log.Printf("AgentTrace %s", version)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Ensure data directory exists
	if *dbPath != ":memory:" {
		dir := filepath.Dir(*dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("error: %v", err)
		}
	}

	// Open database
	store, err := storage.Open(*dbPath)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}
	defer store.Close()
	log.Printf("database: %s", *dbPath)

	// Build handler chain
	apiHandler := api.NewServer(store)

	// Attach OTLP exporter if configured
	if *otlpExport != "" {
		exporter := otel.NewExporter(*otlpExport)
		apiHandler.SetExporter(exporter)
		log.Printf("OTLP export: %s/v1/traces", *otlpExport)
	}

	var handler http.Handler

	if *dev {
		log.Printf("dev mode: proxying / �?http://localhost:5173")
		handler = devProxy(apiHandler)
	} else {
		spaHandler := web.Handler()
		if spaHandler != nil {
			handler = hybridHandler(apiHandler, spaHandler)
			log.Printf("frontend: embedded (%s)", "react SPA")
		} else {
			handler = apiHandler
			log.Printf("frontend: none (API only mode)")
		}
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	log.Printf("listening on http://%s", addr)
	log.Printf("open http://%s to view traces", addr)
	srv := &http.Server{Addr: addr, Handler: handler}
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Printf("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// hybridHandler routes /api/* to the API server and everything else to the SPA.
// For SPA routing (e.g. /trace/abc123), falls back to index.html when the file doesn't exist.
func hybridHandler(apiHandler *api.Server, spa http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			apiHandler.ServeHTTP(w, r)
			return
		}
		// For SPA client-side routing, serve index.html for non-file paths
		// Files are served directly (JS, CSS, favicon, etc.)
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			spa.ServeHTTP(w, r)
			return
		}
		// Try the requested file; if 404, serve index.html (SPA fallback)
		rec := &responseRecorder{header: make(http.Header)}
		spa.ServeHTTP(rec, r)
		if rec.status == 404 {
			r.URL.Path = "/"
			spa.ServeHTTP(w, r)
			return
		}
		for k, vs := range rec.header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(rec.status)
		w.Write(rec.body.Bytes())
	}
}

type responseRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (r *responseRecorder) Header() http.Header         { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *responseRecorder) WriteHeader(status int)       { r.status = status }

// devProxy forwards non-API requests to the Vite dev server.
func devProxy(api *api.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			api.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("http://localhost:5173%s", r.URL.RequestURI()), http.StatusFound)
	}
}
