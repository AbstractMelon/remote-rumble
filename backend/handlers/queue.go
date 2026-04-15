package handlers

import (
	"context"
	"database/sql"
	"net/http"

	"remote-rumble/backend/db"
	"remote-rumble/backend/models"
)

func (a *App) queueSnapshot(ctx context.Context) ([]models.QueueEntry, int, error) {
	rows, err := a.Store.SQL.QueryContext(ctx, `
		SELECT q.user_id, u.username, q.joined_at
		FROM queue q
		JOIN users u ON u.id = q.user_id
		ORDER BY q.joined_at ASC, q.id ASC
	`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries := make([]models.QueueEntry, 0)
	pos := 1
	for rows.Next() {
		var e models.QueueEntry
		e.Position = pos
		if err := rows.Scan(&e.UserID, &e.Username, &e.JoinedAt); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
		pos++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return entries, len(entries), nil
}

func (a *App) HandleQueueGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	entries, total, err := a.queueSnapshot(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load queue")
		return
	}

	public := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		public = append(public, map[string]any{"username": e.Username, "pos": e.Position})
	}
	writeJSON(w, http.StatusOK, map[string]any{"positions": public, "total": total})
}

func (a *App) HandleQueueJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	user, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	inOpenFight, err := isUserInOpenFight(r.Context(), a.Store.SQL, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate current fight state")
		return
	}
	if inOpenFight {
		writeError(w, http.StatusConflict, "cannot join queue while in an active match")
		return
	}

	_, err = a.Store.SQL.ExecContext(r.Context(), `INSERT INTO queue(user_id, joined_at) VALUES(?, unixepoch())`, user.ID)
	if err != nil {
		if db.IsUniqueViolation(err) {
			// Idempotent join: if already queued (e.g. after page reload), return current queue state.
			err = nil
		} else {
			writeError(w, http.StatusInternalServerError, "failed to join queue")
			return
		}
	}

	entries, total, err := a.queueSnapshot(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load queue")
		return
	}

	position := 0
	for _, e := range entries {
		if e.UserID == user.ID {
			position = e.Position
			break
		}
	}
	if position == 0 {
		writeError(w, http.StatusInternalServerError, "failed to determine queue position")
		return
	}

	availableBots := a.Hub.AvailableBotCount(a.Store.SQL, r.Context())
	a.Hub.BroadcastQueueState(a.Store.SQL)

	writeJSON(w, http.StatusOK, map[string]any{
		"position":      position,
		"ahead":         position - 1,
		"total":         total,
		"availableBots": availableBots,
	})
}

func (a *App) HandleQueueLeave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	user, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	result, err := a.Store.SQL.ExecContext(r.Context(), `DELETE FROM queue WHERE user_id = ?`, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to leave queue")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		writeError(w, http.StatusNotFound, "you are not in queue")
		return
	}

	a.Hub.BroadcastQueueState(a.Store.SQL)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func queuePosition(entries []models.QueueEntry, userID int64) int {
	for _, e := range entries {
		if e.UserID == userID {
			return e.Position
		}
	}
	return 0
}

func isInQueue(ctx context.Context, dbx *sql.DB, userID int64) (bool, error) {
	row := dbx.QueryRowContext(ctx, `SELECT 1 FROM queue WHERE user_id = ?`, userID)
	var one int
	err := row.Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func isUserInOpenFight(ctx context.Context, dbx *sql.DB, userID int64) (bool, error) {
	row := dbx.QueryRowContext(ctx, `
		SELECT 1
		FROM fights
		WHERE status IN ('pending', 'selecting', 'active')
		  AND (player1_id = ? OR player2_id = ?)
		ORDER BY id DESC
		LIMIT 1
	`, userID, userID)

	var one int
	err := row.Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
