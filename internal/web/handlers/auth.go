package handlers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/opencity"
)

type AuthHandler struct {
	DB             *sql.DB
	BaseURL        string
	AdminUsernames []string
	SecureCookie   bool
}

func (h *AuthHandler) GetLogin(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "login.html", map[string]any{
		"Error": r.URL.Query().Get("error"),
	})
}

func (h *AuthHandler) PostLogin(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if username == "" || password == "" {
		http.Redirect(w, r, "/login?error=Credenziali+mancanti", http.StatusSeeOther)
		return
	}

	jwt, err := opencity.Login(h.BaseURL, username, password)
	if err != nil {
		http.Redirect(w, r, "/login?error=Credenziali+non+valide", http.StatusSeeOther)
		return
	}

	userID, roles, err := decodeJWTClaims(jwt)
	if err != nil || !hasOperatorRole(roles) {
		http.Redirect(w, r, "/login?error=Accesso+riservato+agli+operatori", http.StatusSeeOther)
		return
	}

	client := opencity.NewClient(h.BaseURL, jwt)
	user, err := client.GetUser(userID)
	if err != nil {
		http.Redirect(w, r, "/login?error=Errore+profilo+operatore", http.StatusSeeOther)
		return
	}

	ruolo := "operator"
	if user.Role == "admin" || h.isAdminUsername(username) {
		ruolo = "admin"
	}

	svcJSON, _ := json.Marshal(user.EnabledServiceIDs)
	sess := &db.Sessione{
		ID:          uuid.NewString(),
		Operatore:   username,
		UserID:      userID,
		JWTOpenCity: jwt,
		Ruolo:       ruolo,
		ServiceIDs:  string(svcJSON),
		ScadeAt:     time.Now().Add(10 * 24 * time.Hour),
		CreatedAt:   time.Now(),
	}
	if err := db.InsertSessione(h.DB, sess); err != nil {
		http.Error(w, "Errore sessione", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sess.ID,
		Path:     "/",
		Expires:  sess.ScadeAt,
		HttpOnly: true,
		Secure:   h.SecureCookie,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *AuthHandler) GetLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_id"); err == nil {
		db.DeleteSessione(h.DB, cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.SecureCookie,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *AuthHandler) isAdminUsername(u string) bool {
	for _, a := range h.AdminUsernames {
		if strings.EqualFold(a, u) {
			return true
		}
	}
	return false
}

func decodeJWTClaims(token string) (userID string, roles []string, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", nil, errInvalidJWT
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, err
	}
	var claims struct {
		ID    string   `json:"id"`
		Roles []string `json:"roles"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", nil, err
	}
	return claims.ID, claims.Roles, nil
}

func hasOperatorRole(roles []string) bool {
	for _, r := range roles {
		if r == "ROLE_OPERATORE" || r == "ROLE_ADMIN" {
			return true
		}
	}
	return false
}

var errInvalidJWT = fmt.Errorf("JWT non valido")
