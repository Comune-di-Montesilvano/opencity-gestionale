package middleware

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"opencity-backend/internal/db"
)

type ctxKey int

const ctxOperatore ctxKey = 0

type OperatorCtx struct {
	SessionID  string
	Username   string
	UserID     string
	JWT        string
	Ruolo      string   // "admin" | "operator"
	ServiceIDs []string
}

func (o *OperatorCtx) IsAdmin() bool { return o.Ruolo == "admin" }

func (o *OperatorCtx) CanAccessService(serviceID string) bool {
	if o.IsAdmin() {
		return true
	}
	for _, id := range o.ServiceIDs {
		if id == serviceID {
			return true
		}
	}
	return false
}

func FromContext(ctx context.Context) *OperatorCtx {
	op, _ := ctx.Value(ctxOperatore).(*OperatorCtx)
	return op
}

func Auth(dbConn *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session_id")
			if err != nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			sess, err := db.GetSessione(dbConn, cookie.Value)
			if err != nil || sess == nil {
				http.SetCookie(w, clearCookie())
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			if time.Now().After(sess.ScadeAt) {
				db.DeleteSessione(dbConn, sess.ID)
				http.SetCookie(w, clearCookie())
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			var svcIDs []string
			json.Unmarshal([]byte(sess.ServiceIDs), &svcIDs)

			op := &OperatorCtx{
				SessionID:  sess.ID,
				Username:   sess.Operatore,
				UserID:     sess.UserID,
				JWT:        sess.JWTOpenCity,
				Ruolo:      sess.Ruolo,
				ServiceIDs: svcIDs,
			}
			ctx := context.WithValue(r.Context(), ctxOperatore, op)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		op := FromContext(r.Context())
		if op == nil || !op.IsAdmin() {
			http.Error(w, "Accesso negato", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clearCookie() *http.Cookie {
	return &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}
