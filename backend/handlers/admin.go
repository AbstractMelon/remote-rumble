package handlers

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type matchCandidate struct {
	UserID    int64
	Username  string
	JoinedAt  int64
	Weight    float64
	QueueAgeS float64
}

type addBotRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type endFightRequest struct {
	WinnerID *int64 `json:"winnerId"`
}

func (a *App) HandleAdminFights(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	rows, err := a.Store.SQL.QueryContext(r.Context(), `
		SELECT f.id,
		       f.player1_id,
		       f.player2_id,
		       IFNULL(u1.username, ''),
		       IFNULL(u2.username, ''),
		       IFNULL(f.bot1_id, ''),
		       IFNULL(f.bot2_id, ''),
		       IFNULL(f.started_at, 0),
		       IFNULL(f.ended_at, 0),
		       IFNULL(f.winner_id, 0),
		       f.status
		FROM fights f
		LEFT JOIN users u1 ON u1.id = f.player1_id
		LEFT JOIN users u2 ON u2.id = f.player2_id
		ORDER BY f.id DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load fights")
		return
	}
	defer rows.Close()

	fights := make([]map[string]any, 0)
	for rows.Next() {
		var fight struct {
			ID        int64
			Player1ID int64
			Player2ID int64
			P1Name    string
			P2Name    string
			Bot1ID    string
			Bot2ID    string
			StartedAt int64
			EndedAt   int64
			WinnerID  int64
			Status    string
		}
		if err := rows.Scan(&fight.ID, &fight.Player1ID, &fight.Player2ID, &fight.P1Name, &fight.P2Name, &fight.Bot1ID, &fight.Bot2ID, &fight.StartedAt, &fight.EndedAt, &fight.WinnerID, &fight.Status); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse fights")
			return
		}
		fights = append(fights, map[string]any{
			"id":          fight.ID,
			"player1Id":   fight.Player1ID,
			"player2Id":   fight.Player2ID,
			"player1Name": fight.P1Name,
			"player2Name": fight.P2Name,
			"bot1Id":      fight.Bot1ID,
			"bot2Id":      fight.Bot2ID,
			"startedAt":   fight.StartedAt,
			"endedAt":     fight.EndedAt,
			"winnerId":    fight.WinnerID,
			"status":      fight.Status,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"fights": fights})
}

func (a *App) HandleAdminMatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	rows, err := a.Store.SQL.QueryContext(r.Context(), `
		SELECT q.user_id, u.username, q.joined_at
		FROM queue q
		JOIN users u ON u.id = q.user_id
		ORDER BY q.joined_at ASC, q.id ASC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load queue")
		return
	}
	defer rows.Close()

	now := float64(time.Now().Unix())
	candidates := make([]matchCandidate, 0)
	for rows.Next() {
		var c matchCandidate
		if err := rows.Scan(&c.UserID, &c.Username, &c.JoinedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse queue")
			return
		}
		c.QueueAgeS = math.Max(1, now-float64(c.JoinedAt))
		c.Weight = c.QueueAgeS * c.QueueAgeS
		candidates = append(candidates, c)
	}

	if len(candidates) < 2 {
		writeError(w, http.StatusBadRequest, "at least 2 users are required in queue")
		return
	}

	p1 := weightedPick(candidates)
	remaining := make([]matchCandidate, 0, len(candidates)-1)
	for _, c := range candidates {
		if c.UserID != p1.UserID {
			remaining = append(remaining, c)
		}
	}
	p2 := weightedPick(remaining)

	tx, err := a.Store.SQL.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create transaction")
		return
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(r.Context(), `
		INSERT INTO fights(player1_id, player2_id, status)
		VALUES(?, ?, 'selecting')
	`, p1.UserID, p2.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create fight")
		return
	}
	fightID, _ := result.LastInsertId()

	if _, err := tx.ExecContext(r.Context(), `DELETE FROM queue WHERE user_id IN (?, ?)`, p1.UserID, p2.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update queue")
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit match")
		return
	}

	a.Hub.SendToUser(p1.UserID, map[string]any{"type": "matched", "fightId": fightID, "opponent": p2.Username})
	a.Hub.SendToUser(p2.UserID, map[string]any{"type": "matched", "fightId": fightID, "opponent": p1.Username})
	a.Hub.BroadcastBotList(a.Store.SQL)
	a.Hub.BroadcastQueueState(a.Store.SQL)

	writeJSON(w, http.StatusOK, map[string]any{
		"fightId": fightID,
		"player1": p1,
		"player2": p2,
	})
}

func weightedPick(candidates []matchCandidate) matchCandidate {
	if len(candidates) == 1 {
		return candidates[0]
	}
	total := 0.0
	for _, c := range candidates {
		total += c.Weight
	}
	if total <= 0 {
		return candidates[rand.Intn(len(candidates))]
	}

	target := rand.Float64() * total
	acc := 0.0
	for _, c := range candidates {
		acc += c.Weight
		if acc >= target {
			return c
		}
	}
	return candidates[len(candidates)-1]
}

func (a *App) HandleAdminFightByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, "/api/admin/fights/")
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(parts) != 2 || parts[1] != "end" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	fightID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || fightID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid fight id")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req endFightRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	var p1, p2 int64
	if err := a.Store.SQL.QueryRowContext(r.Context(), `SELECT player1_id, player2_id FROM fights WHERE id = ?`, fightID).Scan(&p1, &p2); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "fight not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load fight")
		return
	}

	now := time.Now().Unix()
	winnerID := any(nil)
	if req.WinnerID != nil && (*req.WinnerID == p1 || *req.WinnerID == p2) {
		winnerID = *req.WinnerID
	}

	if _, err := a.Store.SQL.ExecContext(r.Context(), `UPDATE fights SET status='ended', ended_at=?, winner_id=? WHERE id=?`, now, winnerID, fightID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to end fight")
		return
	}

	a.Hub.SendToUser(p1, map[string]any{"type": "fight-end", "fightId": fightID, "winnerId": winnerID})
	a.Hub.SendToUser(p2, map[string]any{"type": "fight-end", "fightId": fightID, "winnerId": winnerID})

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) HandleAdminBots(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.HandleBotsGet(w, r)
	case http.MethodPost:
		var req addBotRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.ID = strings.TrimSpace(req.ID)
		req.Name = strings.TrimSpace(req.Name)
		if req.ID == "" || req.Name == "" {
			writeError(w, http.StatusBadRequest, "id and name are required")
			return
		}
		_, err := a.Store.SQL.ExecContext(r.Context(), `INSERT INTO bots(id, name, description, enabled) VALUES(?, ?, ?, 1)`, req.ID, req.Name, req.Description)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				writeError(w, http.StatusBadRequest, "bot id already exists")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to add bot")
			return
		}
		a.Hub.BroadcastBotList(a.Store.SQL)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) HandleAdminBotsByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	suffix := strings.TrimPrefix(r.URL.Path, "/api/admin/bots/")
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	botID := parts[0]

	if len(parts) == 1 && r.Method == http.MethodDelete {
		_, err := a.Store.SQL.ExecContext(r.Context(), `DELETE FROM bots WHERE id = ?`, botID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove bot")
			return
		}
		a.Hub.BroadcastBotList(a.Store.SQL)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	if len(parts) == 2 && (parts[1] == "disable" || parts[1] == "enable") && r.Method == http.MethodPost {
		enabled := 1
		command := "enable"
		if parts[1] == "disable" {
			enabled = 0
			command = "disable"
		}
		if _, err := a.Store.SQL.ExecContext(r.Context(), `UPDATE bots SET enabled = ? WHERE id = ?`, enabled, botID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update bot status")
			return
		}
		a.Hub.SendCommandToBot(botID, map[string]any{"type": "command", "command": command})
		a.Hub.BroadcastBotList(a.Store.SQL)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	writeError(w, http.StatusNotFound, "not found")
}

func (a *App) HandleAdminQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	rows, err := a.Store.SQL.QueryContext(r.Context(), `
		SELECT q.user_id, u.username, q.joined_at
		FROM queue q
		JOIN users u ON u.id = q.user_id
		ORDER BY q.joined_at ASC, q.id ASC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load queue")
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var uid int64
		var username string
		var joined int64
		if err := rows.Scan(&uid, &username, &joined); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse queue")
			return
		}
		items = append(items, map[string]any{"userId": uid, "username": username, "joinedAt": joined})
	}
	writeJSON(w, http.StatusOK, map[string]any{"queue": items})
}

func (a *App) HandleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}

	rows, err := a.Store.SQL.QueryContext(r.Context(), `SELECT id, username, IFNULL(email,''), is_guest, is_admin, created_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load users")
		return
	}
	defer rows.Close()

	users := []map[string]any{}
	for rows.Next() {
		var id, createdAt int64
		var username, email string
		var isGuest, isAdmin int
		if err := rows.Scan(&id, &username, &email, &isGuest, &isAdmin, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse users")
			return
		}
		users = append(users, map[string]any{
			"id":        id,
			"username":  username,
			"email":     email,
			"isGuest":   isGuest == 1,
			"isAdmin":   isAdmin == 1,
			"createdAt": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *App) HandleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req map[string]string
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	allowed := map[string]struct{}{
		"stream_url":         {},
		"fight_duration_sec": {},
		"mediamtx_whep_url":  {},
		"username_blocklist": {},
	}

	tx, err := a.Store.SQL.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()

	for key, value := range req {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update settings")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) HandleAdminDBView(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	suffix := strings.TrimPrefix(r.URL.Path, "/api/admin/db/")
	table := strings.Trim(strings.Split(suffix, "/")[0], " ")
	allowed := map[string]struct{}{"users": {}, "fights": {}, "bots": {}, "queue": {}}
	if _, ok := allowed[table]; !ok {
		writeError(w, http.StatusBadRequest, "table not allowed")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const perPage = 25
	offset := (page - 1) * perPage

	query := fmt.Sprintf("SELECT * FROM %s ORDER BY rowid DESC LIMIT ? OFFSET ?", table)
	rows, err := a.Store.SQL.QueryContext(r.Context(), query, perPage, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query table")
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read columns")
		return
	}

	results := make([]map[string]any, 0)
	for rows.Next() {
		scan := make([]any, len(columns))
		target := make([]any, len(columns))
		for i := range scan {
			target[i] = &scan[i]
		}
		if err := rows.Scan(target...); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan rows")
			return
		}
		rowMap := map[string]any{}
		for i, col := range columns {
			raw := scan[i]
			switch v := raw.(type) {
			case []byte:
				rowMap[col] = string(v)
			default:
				rowMap[col] = v
			}
		}
		results = append(results, rowMap)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"table": table,
		"page":  page,
		"rows":  results,
	})
}
