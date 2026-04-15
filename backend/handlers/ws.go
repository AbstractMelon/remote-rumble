package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type clientConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
	userID  int64
	isAdmin bool
}

type botConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
	botID   string
}

type Hub struct {
	mu      sync.RWMutex
	clients map[int64]map[*clientConn]struct{}
	all     map[*clientConn]struct{}
	bots    map[string]*botConn
}

func NewHub() *Hub {
	return &Hub{
		clients: map[int64]map[*clientConn]struct{}{},
		all:     map[*clientConn]struct{}{},
		bots:    map[string]*botConn{},
	}
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (h *Hub) HandleWS(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := strings.TrimSpace(r.URL.Query().Get("role"))
		if role == "bot" {
			h.handleBotWS(app, w, r)
			return
		}
		h.handleClientWS(app, w, r)
	}
}

func (h *Hub) handleClientWS(app *App, w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		token = readTokenFromRequest(r)
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	user, err := app.userByToken(r.Context(), token)
	if err != nil {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid token"), time.Now().Add(2*time.Second))
		_ = conn.Close()
		return
	}

	conn.SetReadLimit(64 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	cc := &clientConn{conn: conn, userID: user.ID, isAdmin: user.IsAdmin}
	h.registerClient(cc)
	defer h.unregisterClient(cc)

	h.sendQueueStateToClient(app.Store.SQL, cc)
	h.sendBotListToClient(app.Store.SQL, cc)

	for {
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}

		var msg map[string]any
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}
		mType, _ := msg["type"].(string)
		switch mType {
		case "ping":
			h.writeClientJSON(cc, map[string]any{"type": "pong", "t": msg["t"]})
		case "control":
			h.relayControl(app, cc.userID, msg)
		}
	}
}

func (h *Hub) handleBotWS(app *App, w http.ResponseWriter, r *http.Request) {
	botID := strings.TrimSpace(r.URL.Query().Get("botId"))
	if botID == "" {
		http.Error(w, "missing botId", http.StatusBadRequest)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(128 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	bc := &botConn{conn: conn, botID: botID}
	h.registerBot(bc)
	h.BroadcastBotList(app.Store.SQL)
	defer func() {
		h.unregisterBot(botID, bc)
		h.BroadcastBotList(app.Store.SQL)
	}()

	var enabled int
	if err := app.Store.SQL.QueryRowContext(r.Context(), `SELECT enabled FROM bots WHERE id = ?`, botID).Scan(&enabled); err == nil && enabled == 0 {
		h.writeBotJSON(bc, map[string]any{"type": "command", "command": "disable"})
	}
	for {
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}

		var msg map[string]any
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}
		mType, _ := msg["type"].(string)
		switch mType {
		case "telemetry":
			h.forwardTelemetry(app.Store.SQL, botID, msg)
		case "ping":
			h.writeBotJSON(bc, map[string]any{"type": "pong", "t": msg["t"]})
		}
	}
}

func (h *Hub) relayControl(app *App, userID int64, msg map[string]any) {
	botID, ok := activeBotForUser(app.Store.SQL, userID)
	if !ok {
		return
	}

	h.mu.RLock()
	bot, online := h.bots[botID]
	h.mu.RUnlock()
	if !online {
		return
	}

	msg["type"] = "control"
	msg["botId"] = botID
	h.writeBotJSON(bot, msg)
}

func activeBotForUser(sqlDB *sql.DB, userID int64) (string, bool) {
	row := sqlDB.QueryRow(`
		SELECT CASE WHEN player1_id = ? THEN bot1_id ELSE bot2_id END AS bot_id
		FROM fights
		WHERE status = 'active' AND (player1_id = ? OR player2_id = ?)
		ORDER BY id DESC
		LIMIT 1
	`, userID, userID, userID)
	var botID sql.NullString
	if err := row.Scan(&botID); err != nil {
		return "", false
	}
	if !botID.Valid || strings.TrimSpace(botID.String) == "" {
		return "", false
	}
	return strings.TrimSpace(botID.String), true
}

func (h *Hub) forwardTelemetry(sqlDB *sql.DB, botID string, msg map[string]any) {
	row := sqlDB.QueryRow(`
		SELECT player1_id, player2_id, bot1_id, bot2_id
		FROM fights
		WHERE status = 'active' AND (bot1_id = ? OR bot2_id = ?)
		ORDER BY id DESC
		LIMIT 1
	`, botID, botID)

	var p1, p2 int64
	var b1, b2 sql.NullString
	if err := row.Scan(&p1, &p2, &b1, &b2); err != nil {
		return
	}

	if b1.Valid && b1.String == botID {
		h.SendToUser(p1, msg)
	}
	if b2.Valid && b2.String == botID {
		h.SendToUser(p2, msg)
	}
}

func (h *Hub) registerClient(c *clientConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c.userID]; !ok {
		h.clients[c.userID] = map[*clientConn]struct{}{}
	}
	h.clients[c.userID][c] = struct{}{}
	h.all[c] = struct{}{}
}

func (h *Hub) unregisterClient(c *clientConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.all, c)
	if set, ok := h.clients[c.userID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, c.userID)
		}
	}
	_ = c.conn.Close()
}

func (h *Hub) registerBot(b *botConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.bots[b.botID]; ok {
		_ = old.conn.Close()
	}
	h.bots[b.botID] = b
}

func (h *Hub) unregisterBot(botID string, b *botConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if current, ok := h.bots[botID]; ok && current == b {
		delete(h.bots, botID)
	}
	_ = b.conn.Close()
}

func (h *Hub) writeClientJSON(c *clientConn, payload any) bool {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.WriteJSON(payload); err != nil {
		return false
	}
	return true
}

func (h *Hub) writeBotJSON(b *botConn, payload any) bool {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	if err := b.conn.WriteJSON(payload); err != nil {
		return false
	}
	return true
}

func (h *Hub) SendToUser(userID int64, payload any) {
	h.mu.RLock()
	conns := make([]*clientConn, 0)
	for c := range h.clients[userID] {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		h.writeClientJSON(c, payload)
	}
}

func (h *Hub) Broadcast(payload any) {
	h.mu.RLock()
	conns := make([]*clientConn, 0, len(h.all))
	for c := range h.all {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		h.writeClientJSON(c, payload)
	}
}

func (h *Hub) SendCommandToBot(botID string, payload any) {
	h.mu.RLock()
	bot, ok := h.bots[botID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	h.writeBotJSON(bot, payload)
}

func (h *Hub) IsBotOnline(botID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.bots[botID]
	return ok
}

func (h *Hub) AvailableBotCount(sqlDB *sql.DB, ctx context.Context) int {
	rows, err := sqlDB.QueryContext(ctx, `SELECT id, enabled FROM bots`)
	if err != nil {
		return 0
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id string
		var enabled int
		if err := rows.Scan(&id, &enabled); err != nil {
			continue
		}
		if enabled == 1 && h.IsBotOnline(id) {
			count++
		}
	}
	return count
}

func (h *Hub) BroadcastQueueState(sqlDB *sql.DB) {
	rows, err := sqlDB.Query(`
		SELECT u.username
		FROM queue q
		JOIN users u ON u.id = q.user_id
		ORDER BY q.joined_at ASC, q.id ASC
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	positions := make([]map[string]any, 0)
	pos := 1
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			continue
		}
		positions = append(positions, map[string]any{"username": username, "pos": pos})
		pos++
	}

	h.Broadcast(map[string]any{"type": "queue", "positions": positions, "total": len(positions)})
}

func (h *Hub) sendQueueStateToClient(sqlDB *sql.DB, c *clientConn) {
	rows, err := sqlDB.Query(`
		SELECT u.username
		FROM queue q
		JOIN users u ON u.id = q.user_id
		ORDER BY q.joined_at ASC, q.id ASC
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	positions := make([]map[string]any, 0)
	pos := 1
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			continue
		}
		positions = append(positions, map[string]any{"username": username, "pos": pos})
		pos++
	}

	h.writeClientJSON(c, map[string]any{"type": "queue", "positions": positions, "total": len(positions)})
}

func (h *Hub) BroadcastBotList(sqlDB *sql.DB) {
	bots := h.botList(sqlDB)
	h.Broadcast(map[string]any{"type": "bot-list", "bots": bots})
}

func (h *Hub) sendBotListToClient(sqlDB *sql.DB, c *clientConn) {
	bots := h.botList(sqlDB)
	h.writeClientJSON(c, map[string]any{"type": "bot-list", "bots": bots})
}

func (h *Hub) botList(sqlDB *sql.DB) []map[string]any {
	rows, err := sqlDB.Query(`SELECT id, name, IFNULL(description,''), enabled FROM bots ORDER BY name ASC`)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()

	list := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, desc string
		var enabled int
		if err := rows.Scan(&id, &name, &desc, &enabled); err != nil {
			continue
		}
		list = append(list, map[string]any{
			"id":          id,
			"name":        name,
			"description": desc,
			"enabled":     enabled == 1,
			"online":      h.IsBotOnline(id),
		})
	}
	return list
}
