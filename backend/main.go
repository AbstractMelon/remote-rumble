package main

import (
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"remote-rumble/backend/db"
	"remote-rumble/backend/handlers"

	"github.com/joho/godotenv"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	loadDotEnv()

	host := envOr("HOST", "0.0.0.0")
	port := envOr("PORT", "8080")
	dbPath := envOr("DB_PATH", "./data/remote-rumble.db")
	sessionSecret := envOr("SESSION_SECRET", "changeme")
	defaultWhep := envOr("MEDIAMTX_WHEP_URL", "http://localhost:8889")
	adminKey := envOr("ADMIN_KEY", "")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("failed to prepare db dir: %v", err)
	}

	store, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer store.Close()

	hub := handlers.NewHub()
	app := &handlers.App{
		Store:              store,
		Hub:                hub,
		SessionSecret:      []byte(sessionSecret),
		SessionTTL:         24 * time.Hour,
		DefaultMediamtxURL: defaultWhep,
		AdminKey:           adminKey,
	}

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			handlers.DeleteExpiredSessions(store.SQL)
		}
	}()

	mux := http.NewServeMux()
	registerRoutes(mux, app)

	addr := host + ":" + port
	log.Printf("Remote Rumble server listening on %s", addr)
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}

func registerRoutes(mux *http.ServeMux, app *handlers.App) {
	mux.HandleFunc("/ws", app.Hub.HandleWS(app))

	mux.HandleFunc("/api/auth/register", app.HandleRegister)
	mux.HandleFunc("/api/auth/login", app.HandleLogin)
	mux.HandleFunc("/api/auth/guest", app.HandleGuest)
	mux.HandleFunc("/api/auth/logout", app.HandleLogout)
	mux.HandleFunc("/api/auth/me", app.HandleMe)

	mux.HandleFunc("/api/queue/join", app.HandleQueueJoin)
	mux.HandleFunc("/api/queue/leave", app.HandleQueueLeave)
	mux.HandleFunc("/api/queue", app.HandleQueueGet)

	mux.HandleFunc("/api/fights", app.HandleFightsList)
	mux.HandleFunc("/api/fights/", app.HandleFightByID)

	mux.HandleFunc("/api/bots", app.HandleBotsGet)
	mux.HandleFunc("/api/settings", app.HandleSettingsGet)
	mux.HandleFunc("/api/stream-config", app.HandleStreamConfigGet)

	mux.HandleFunc("/api/admin/match", app.HandleAdminMatch)
	mux.HandleFunc("/api/admin/verify", app.HandleAdminVerify)
	mux.HandleFunc("/api/admin/fights", app.HandleAdminFights)
	mux.HandleFunc("/api/admin/fights/", app.HandleAdminFightByID)
	mux.HandleFunc("/api/admin/bots", app.HandleAdminBots)
	mux.HandleFunc("/api/admin/bots/", app.HandleAdminBotsByID)
	mux.HandleFunc("/api/admin/queue", app.HandleAdminQueue)
	mux.HandleFunc("/api/admin/users", app.HandleAdminUsers)
	mux.HandleFunc("/api/admin/settings", app.HandleAdminSettings)
	mux.HandleFunc("/api/admin/db/", app.HandleAdminDBView)

	webRoot, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		log.Fatalf("failed to setup embedded web: %v", err)
	}
	static := http.FileServer(http.FS(webRoot))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/ws" {
			http.NotFound(w, r)
			return
		}

		if r.URL.Path != "/" {
			clean := strings.TrimPrefix(pathClean(r.URL.Path), "/")
			if clean != "" {
				if _, err := fs.Stat(webRoot, clean); err == nil {
					static.ServeHTTP(w, r)
					return
				}
			}
		}

		index, err := fs.ReadFile(webRoot, "index.html")
		if err != nil {
			http.Error(w, "index not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func envOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func loadDotEnv() {
	paths := []string{".env", "../.env"}
	loaded := make([]string, 0, len(paths))

	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := godotenv.Load(path); err != nil {
			log.Printf("warning: failed loading %s: %v", path, err)
			continue
		}
		loaded = append(loaded, path)
	}

	if len(loaded) > 0 {
		log.Printf("loaded env file(s): %s", strings.Join(loaded, ", "))
	}
}

func pathClean(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return filepath.ToSlash(filepath.Clean(p))
}
