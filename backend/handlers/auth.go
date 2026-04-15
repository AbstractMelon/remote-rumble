package handlers

import (
	"database/sql"
	"net/http"
	"regexp"
	"strings"

	"remote-rumble/backend/db"
	"remote-rumble/backend/models"

	"golang.org/x/crypto/bcrypt"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,20}$`)

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type guestRequest struct {
	Username string `json:"username"`
}

func (a *App) validateUsername(r *http.Request, username string) string {
	trimmed := strings.TrimSpace(username)
	if len(trimmed) < 3 || len(trimmed) > 20 {
		return "username must be 3-20 characters"
	}
	if !usernameRe.MatchString(trimmed) {
		return "username may only contain letters, numbers, underscores, and hyphens"
	}

	row := a.Store.SQL.QueryRowContext(r.Context(), `SELECT value FROM settings WHERE key = 'username_blocklist'`)
	var raw string
	if err := row.Scan(&raw); err == nil {
		blocked := map[string]struct{}{}
		for _, line := range strings.Split(raw, "\n") {
			entry := strings.ToLower(strings.TrimSpace(line))
			if entry != "" {
				blocked[entry] = struct{}{}
			}
		}
		if _, found := blocked[strings.ToLower(trimmed)]; found {
			return "username is not allowed"
		}
	}

	return ""
}

func (a *App) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "rr_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
		MaxAge:   int(a.SessionTTL.Seconds()),
	})
}

func (a *App) writeAuthResponse(w http.ResponseWriter, token string, user models.User) {
	a.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}

func (a *App) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if msg := a.validateUsername(r, req.Username); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	result, err := a.Store.SQL.ExecContext(r.Context(), `
		INSERT INTO users(username, email, password_hash, is_guest, is_admin)
		VALUES(?, ?, ?, 0, 0)
	`, req.Username, req.Email, string(hash))
	if err != nil {
		if db.IsUniqueViolation(err) {
			if strings.Contains(strings.ToLower(err.Error()), "username") {
				writeError(w, http.StatusBadRequest, "username is already taken")
				return
			}
			writeError(w, http.StatusBadRequest, "email is already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	userID, _ := result.LastInsertId()
	token, expiresAt, err := a.newSessionToken(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	if _, err := a.Store.SQL.ExecContext(r.Context(), `INSERT INTO sessions(token, user_id, expires_at) VALUES(?, ?, ?)`, token, userID, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist session")
		return
	}

	a.writeAuthResponse(w, token, models.User{ID: userID, Username: req.Username, Email: req.Email, IsGuest: false, IsAdmin: false})
}

func (a *App) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	row := a.Store.SQL.QueryRowContext(r.Context(), `
		SELECT id, username, email, password_hash, is_admin, is_guest
		FROM users
		WHERE email = ?
	`, req.Email)

	var user models.User
	var hash string
	var isAdmin, isGuest int
	if err := row.Scan(&user.ID, &user.Username, &user.Email, &hash, &isAdmin, &isGuest); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}

	if isGuest == 1 {
		writeError(w, http.StatusUnauthorized, "guest users cannot log in with email/password")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	user.IsAdmin = isAdmin == 1
	user.IsGuest = isGuest == 1
	token, expiresAt, err := a.newSessionToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	if _, err := a.Store.SQL.ExecContext(r.Context(), `INSERT INTO sessions(token, user_id, expires_at) VALUES(?, ?, ?)`, token, user.ID, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist session")
		return
	}

	a.writeAuthResponse(w, token, user)
}

func (a *App) HandleGuest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req guestRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if msg := a.validateUsername(r, req.Username); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	result, err := a.Store.SQL.ExecContext(r.Context(), `
		INSERT INTO users(username, is_guest, is_admin)
		VALUES(?, 1, 0)
	`, req.Username)
	if err != nil {
		if db.IsUniqueViolation(err) {
			writeError(w, http.StatusBadRequest, "username is already taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create guest user")
		return
	}

	userID, _ := result.LastInsertId()
	token, expiresAt, err := a.newSessionToken(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	if _, err := a.Store.SQL.ExecContext(r.Context(), `INSERT INTO sessions(token, user_id, expires_at) VALUES(?, ?, ?)`, token, userID, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist session")
		return
	}

	a.writeAuthResponse(w, token, models.User{ID: userID, Username: req.Username, IsGuest: true, IsAdmin: false})
}

func (a *App) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	token := readTokenFromRequest(r)
	if token != "" {
		_, _ = a.Store.SQL.ExecContext(r.Context(), `DELETE FROM sessions WHERE token = ?`, token)
	}

	http.SetCookie(w, &http.Cookie{Name: "rr_token", Value: "", MaxAge: -1, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	user, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	row := a.Store.SQL.QueryRowContext(r.Context(), `SELECT id, username, IFNULL(email, ''), is_admin, is_guest, created_at FROM users WHERE id = ?`, user.ID)
	var out models.User
	var isAdmin, isGuest int
	if err := row.Scan(&out.ID, &out.Username, &out.Email, &isAdmin, &isGuest, &out.CreatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}
	out.IsAdmin = isAdmin == 1
	out.IsGuest = isGuest == 1
	writeJSON(w, http.StatusOK, map[string]any{"user": out})
}
