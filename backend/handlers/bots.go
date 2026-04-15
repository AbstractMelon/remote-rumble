package handlers

import (
	"net/http"
	"strings"
)

func (a *App) HandleBotsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, err := a.Store.SQL.QueryContext(r.Context(), `
		SELECT id, name, IFNULL(description,''), enabled, created_at
		FROM bots
		ORDER BY created_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load bots")
		return
	}
	defer rows.Close()

	bots := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, desc string
		var enabled int
		var createdAt int64
		if err := rows.Scan(&id, &name, &desc, &enabled, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse bots")
			return
		}
		bots = append(bots, map[string]any{
			"id":          id,
			"name":        name,
			"description": desc,
			"enabled":     enabled == 1,
			"online":      a.Hub.IsBotOnline(id),
			"createdAt":   createdAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"bots": bots})
}

func (a *App) HandleSettingsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, err := a.Store.SQL.QueryContext(r.Context(), `SELECT key, value FROM settings`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	defer rows.Close()

	settings := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse settings")
			return
		}
		settings[key] = value
	}

	writeJSON(w, http.StatusOK, settings)
}

func (a *App) HandleStreamConfigGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	row := a.Store.SQL.QueryRowContext(r.Context(), `SELECT value FROM settings WHERE key = 'mediamtx_whep_url'`)
	var base string
	if err := row.Scan(&base); err != nil || strings.TrimSpace(base) == "" {
		base = a.DefaultMediamtxURL
	}
	base = strings.TrimSuffix(strings.TrimSpace(base), "/")
	if base == "" {
		base = "http://localhost:8889"
	}
	writeJSON(w, http.StatusOK, map[string]string{"whepUrl": base + "/arena/whep"})
}
