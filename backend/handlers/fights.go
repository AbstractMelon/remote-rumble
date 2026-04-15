package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type botSelectionRequest struct {
	BotID string `json:"botId"`
}

func (a *App) HandleFightsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	user, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	rows, err := a.Store.SQL.QueryContext(r.Context(), `
		SELECT f.id,
		       f.player1_id,
		       f.player2_id,
		       u1.username,
		       u2.username,
		       IFNULL(f.bot1_id, ''),
		       IFNULL(f.bot2_id, ''),
		       IFNULL(f.started_at, 0),
		       IFNULL(f.ended_at, 0),
		       IFNULL(f.winner_id, 0),
		       f.status
		FROM fights f
		LEFT JOIN users u1 ON u1.id = f.player1_id
		LEFT JOIN users u2 ON u2.id = f.player2_id
		WHERE f.player1_id = ? OR f.player2_id = ?
		ORDER BY f.id DESC
	`, user.ID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load fights")
		return
	}
	defer rows.Close()

	fights := make([]map[string]any, 0)
	for rows.Next() {
		var f struct {
			ID        int64
			P1ID      int64
			P2ID      int64
			P1Name    string
			P2Name    string
			Bot1ID    string
			Bot2ID    string
			StartedAt int64
			EndedAt   int64
			WinnerID  int64
			Status    string
		}
		if err := rows.Scan(&f.ID, &f.P1ID, &f.P2ID, &f.P1Name, &f.P2Name, &f.Bot1ID, &f.Bot2ID, &f.StartedAt, &f.EndedAt, &f.WinnerID, &f.Status); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse fights")
			return
		}
		fights = append(fights, map[string]any{
			"id":          f.ID,
			"player1Id":   f.P1ID,
			"player2Id":   f.P2ID,
			"player1Name": f.P1Name,
			"player2Name": f.P2Name,
			"bot1Id":      f.Bot1ID,
			"bot2Id":      f.Bot2ID,
			"startedAt":   f.StartedAt,
			"endedAt":     f.EndedAt,
			"winnerId":    f.WinnerID,
			"status":      f.Status,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"fights": fights})
}

func (a *App) HandleFightByID(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/fights/")
	if suffix == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	fightID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || fightID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid fight id")
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		a.handleFightDetail(w, r, fightID)
		return
	}
	if len(parts) == 2 && parts[1] == "bot" && r.Method == http.MethodPost {
		a.handleFightBotSelection(w, r, fightID)
		return
	}

	writeError(w, http.StatusNotFound, "not found")
}

func (a *App) handleFightDetail(w http.ResponseWriter, r *http.Request, fightID int64) {
	user, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	row := a.Store.SQL.QueryRowContext(r.Context(), `
		SELECT f.id,
		       f.player1_id,
		       f.player2_id,
		       u1.username,
		       u2.username,
		       IFNULL(f.bot1_id, ''),
		       IFNULL(f.bot2_id, ''),
		       IFNULL(f.started_at, 0),
		       IFNULL(f.ended_at, 0),
		       IFNULL(f.winner_id, 0),
		       f.status
		FROM fights f
		LEFT JOIN users u1 ON u1.id = f.player1_id
		LEFT JOIN users u2 ON u2.id = f.player2_id
		WHERE f.id = ?
	`, fightID)

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

	if err := row.Scan(&fight.ID, &fight.Player1ID, &fight.Player2ID, &fight.P1Name, &fight.P2Name, &fight.Bot1ID, &fight.Bot2ID, &fight.StartedAt, &fight.EndedAt, &fight.WinnerID, &fight.Status); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "fight not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load fight")
		return
	}

	if fight.Player1ID != user.ID && fight.Player2ID != user.ID && !user.IsAdmin {
		writeError(w, http.StatusForbidden, "not allowed to view this fight")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"fight": map[string]any{
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
	}})
}

func (a *App) handleFightBotSelection(w http.ResponseWriter, r *http.Request, fightID int64) {
	user, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	var req botSelectionRequest
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.BotID) == "" {
		writeError(w, http.StatusBadRequest, "botId is required")
		return
	}
	req.BotID = strings.TrimSpace(req.BotID)

	var fight struct {
		Player1ID int64
		Player2ID int64
		Bot1ID    string
		Bot2ID    string
		Status    string
	}
	row := a.Store.SQL.QueryRowContext(r.Context(), `SELECT player1_id, player2_id, IFNULL(bot1_id,''), IFNULL(bot2_id,''), status FROM fights WHERE id = ?`, fightID)
	if err := row.Scan(&fight.Player1ID, &fight.Player2ID, &fight.Bot1ID, &fight.Bot2ID, &fight.Status); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "fight not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load fight")
		return
	}

	if fight.Status != "selecting" && fight.Status != "pending" {
		writeError(w, http.StatusBadRequest, "fight is not accepting bot selection")
		return
	}

	if user.ID != fight.Player1ID && user.ID != fight.Player2ID {
		writeError(w, http.StatusForbidden, "you are not in this fight")
		return
	}

	var enabled int
	if err := a.Store.SQL.QueryRowContext(r.Context(), `SELECT enabled FROM bots WHERE id = ?`, req.BotID).Scan(&enabled); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusBadRequest, "selected bot does not exist")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load bot")
		return
	}
	if enabled == 0 {
		writeError(w, http.StatusBadRequest, "selected bot is disabled")
		return
	}
	if !a.Hub.IsBotOnline(req.BotID) {
		writeError(w, http.StatusBadRequest, "selected bot is offline")
		return
	}

	var err error
	var saveResult sql.Result
	if user.ID == fight.Player1ID {
		saveResult, err = a.Store.SQL.ExecContext(r.Context(), `
			UPDATE fights
			SET bot1_id = ?, status = 'selecting'
			WHERE id = ?
			  AND player1_id = ?
			  AND status IN ('selecting', 'pending')
			  AND (IFNULL(bot2_id, '') = '' OR bot2_id <> ?)
		`, req.BotID, fightID, user.ID, req.BotID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save bot selection")
			return
		}
	} else {
		saveResult, err = a.Store.SQL.ExecContext(r.Context(), `
			UPDATE fights
			SET bot2_id = ?, status = 'selecting'
			WHERE id = ?
			  AND player2_id = ?
			  AND status IN ('selecting', 'pending')
			  AND (IFNULL(bot1_id, '') = '' OR bot1_id <> ?)
		`, req.BotID, fightID, user.ID, req.BotID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save bot selection")
			return
		}
	}

	savedRows, _ := saveResult.RowsAffected()
	if savedRows == 0 {
		var status, bot1, bot2 string
		if err := a.Store.SQL.QueryRowContext(r.Context(), `SELECT status, IFNULL(bot1_id,''), IFNULL(bot2_id,'') FROM fights WHERE id = ?`, fightID).Scan(&status, &bot1, &bot2); err != nil {
			writeError(w, http.StatusBadRequest, "failed to save bot selection")
			return
		}
		if status != "selecting" && status != "pending" {
			writeError(w, http.StatusBadRequest, "fight is not accepting bot selection")
			return
		}
		if (user.ID == fight.Player1ID && bot2 == req.BotID) || (user.ID == fight.Player2ID && bot1 == req.BotID) {
			writeError(w, http.StatusBadRequest, "selected bot is already used by your opponent")
			return
		}
		writeError(w, http.StatusBadRequest, "failed to save bot selection")
		return
	}

	now := time.Now().Unix()
	startResult, err := a.Store.SQL.ExecContext(r.Context(), `
		UPDATE fights
		SET status = 'active', started_at = ?
		WHERE id = ?
		  AND status IN ('selecting', 'pending')
		  AND IFNULL(bot1_id, '') <> ''
		  AND IFNULL(bot2_id, '') <> ''
	`, now, fightID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start fight")
		return
	}

	startRows, _ := startResult.RowsAffected()
	started := startRows > 0
	duration := a.getFightDurationSec(r.Context())

	if started {
		var p1ID, p2ID int64
		var bot1ID, bot2ID string
		if err := a.Store.SQL.QueryRowContext(r.Context(), `SELECT player1_id, player2_id, IFNULL(bot1_id,''), IFNULL(bot2_id,'') FROM fights WHERE id = ?`, fightID).Scan(&p1ID, &p2ID, &bot1ID, &bot2ID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load started fight")
			return
		}
		serverTime := time.Now().Unix()
		a.Hub.SendToUser(p1ID, map[string]any{"type": "fight-start", "fightId": fightID, "botId": bot1ID, "durationSec": duration, "serverTime": serverTime})
		a.Hub.SendToUser(p2ID, map[string]any{"type": "fight-start", "fightId": fightID, "botId": bot2ID, "durationSec": duration, "serverTime": serverTime})
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "started": started})
}

func (a *App) getFightDurationSec(ctx context.Context) int {
	row := a.Store.SQL.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'fight_duration_sec'`)
	var raw string
	if err := row.Scan(&raw); err != nil {
		return 180
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 180
	}
	return value
}
