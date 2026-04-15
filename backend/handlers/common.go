package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"remote-rumble/backend/db"
	"remote-rumble/backend/middleware"
)

type App struct {
	Store              *db.Store
	Hub                *Hub
	SessionSecret      []byte
	SessionTTL         time.Duration
	DefaultMediamtxURL string
	AdminKey           string
}

type adminVerifyRequest struct {
	Key string `json:"key"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func readTokenFromRequest(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}

	if cookie, err := r.Cookie("rr_token"); err == nil {
		return strings.TrimSpace(cookie.Value)
	}

	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token
	}

	return ""
}

func (a *App) tokenSignature(payload string) string {
	h := hmac.New(sha256.New, a.SessionSecret)
	_, _ = h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

func (a *App) newSessionToken(userID int64) (string, int64, error) {
	rnd := make([]byte, 16)
	if _, err := rand.Read(rnd); err != nil {
		return "", 0, err
	}
	now := time.Now().Unix()
	expiresAt := time.Now().Add(a.SessionTTL).Unix()
	payload := fmt.Sprintf("%d:%d:%s", userID, now, hex.EncodeToString(rnd))
	sig := a.tokenSignature(payload)
	return payload + "." + sig, expiresAt, nil
}

func (a *App) validateTokenSignature(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	expected := a.tokenSignature(parts[0])
	return hmac.Equal([]byte(expected), []byte(parts[1]))
}

func (a *App) userByToken(ctx context.Context, token string) (middleware.AuthUser, error) {
	if token == "" {
		return middleware.AuthUser{}, errors.New("missing token")
	}
	if !a.validateTokenSignature(token) {
		return middleware.AuthUser{}, errors.New("invalid token")
	}

	row := a.Store.SQL.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.is_admin, u.is_guest
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > unixepoch()
	`, token)

	var user middleware.AuthUser
	var isAdmin, isGuest int
	if err := row.Scan(&user.ID, &user.Username, &isAdmin, &isGuest); err != nil {
		return middleware.AuthUser{}, err
	}
	user.IsAdmin = isAdmin == 1
	user.IsGuest = isGuest == 1
	return user, nil
}

func (a *App) requireAuth(w http.ResponseWriter, r *http.Request) (middleware.AuthUser, bool) {
	token := readTokenFromRequest(r)
	user, err := a.userByToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return middleware.AuthUser{}, false
	}
	ctx := middleware.WithUser(r.Context(), user)
	*r = *r.WithContext(ctx)
	return user, true
}

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) (middleware.AuthUser, bool) {
	adminKey := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	if a.AdminKey != "" {
		if !a.adminKeyMatches(adminKey) {
			writeError(w, http.StatusForbidden, "admin key required")
			return middleware.AuthUser{}, false
		}
		return middleware.AuthUser{Username: "admin-key", IsAdmin: true}, true
	}

	user, ok := a.requireAuth(w, r)
	if !ok {
		return middleware.AuthUser{}, false
	}
	if !user.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return middleware.AuthUser{}, false
	}
	return user, true
}

func (a *App) adminKeyMatches(value string) bool {
	if a.AdminKey == "" {
		return false
	}
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return false
	}
	return hmac.Equal([]byte(candidate), []byte(a.AdminKey))
}

func (a *App) HandleAdminVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if a.AdminKey != "" {
		candidate := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
		if candidate == "" && r.ContentLength > 0 {
			var req adminVerifyRequest
			if err := decodeJSON(r, &req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			candidate = strings.TrimSpace(req.Key)
		}

		if !a.adminKeyMatches(candidate) {
			writeError(w, http.StatusForbidden, "admin key required")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "requiresKey": true})
		return
	}

	user, ok := a.requireAuth(w, r)
	if !ok {
		return
	}
	if !user.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "requiresKey": false})
}

func pathSuffix(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}

func parseInt64Param(raw string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New("invalid id")
	}
	return value, nil
}

func DeleteExpiredSessions(sqlDB *sql.DB) {
	_, _ = sqlDB.ExecContext(context.Background(), `DELETE FROM sessions WHERE expires_at <= unixepoch()`) // best-effort cleanup
}
