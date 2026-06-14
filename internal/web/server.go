package web

import (
	"database/sql"
	"net/http"

	"opencity-gestionale/internal/config"
	"opencity-gestionale/internal/web/handlers"
	"opencity-gestionale/internal/web/middleware"
)

func NewServer(cfg *config.Config, dbConn *sql.DB) http.Handler {
	auth := &handlers.AuthHandler{
		DB:             dbConn,
		BaseURL:        cfg.OpenCityBaseURL,
		AdminUsernames: cfg.AdminUsernames,
		SecureCookie:   cfg.TrustProxy,
	}
	dashboard := &handlers.DashboardHandler{DB: dbConn}
	motori := &handlers.MotoriHandler{DB: dbConn, BaseURL: cfg.OpenCityBaseURL}
	grad := &handlers.GraduatoriaHandler{DB: dbConn, BaseURL: cfg.OpenCityBaseURL}
	acts := &handlers.ActionsHandler{DB: dbConn, BaseURL: cfg.OpenCityBaseURL}
	auditH := &handlers.AuditHandler{DB: dbConn}

	authMW := middleware.Auth(dbConn)

	mux := http.NewServeMux()

	// Health check (non autenticato, usato da Docker e load balancer)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Pubbliche
	mux.HandleFunc("GET /login", auth.GetLogin)
	mux.HandleFunc("POST /login", auth.PostLogin)
	mux.HandleFunc("GET /logout", auth.GetLogout)

	// Statici
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Protette
	mux.Handle("GET /", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	})))
	mux.Handle("GET /dashboard", authMW(http.HandlerFunc(dashboard.GetDashboard)))

	// Motori — lista e wizard (admin only per modifica)
	mux.Handle("GET /motori", authMW(http.HandlerFunc(motori.GetLista)))
	mux.Handle("GET /motori/nuovo", authMW(middleware.RequireAdmin(http.HandlerFunc(motori.GetNuovo))))
	mux.Handle("POST /motori/wizard/connetti", authMW(middleware.RequireAdmin(http.HandlerFunc(motori.PostConnettiServizi))))
	mux.Handle("POST /motori/wizard/crea", authMW(middleware.RequireAdmin(http.HandlerFunc(motori.PostCreaMotore))))
	mux.Handle("GET /motori/{id}/wizard/{step}", authMW(middleware.RequireAdmin(http.HandlerFunc(motori.GetWizardStep))))
	mux.Handle("POST /motori/{id}/wizard/{step}", authMW(middleware.RequireAdmin(http.HandlerFunc(motori.PostWizardStep))))
	mux.Handle("POST /motori/{id}/wizard/test", authMW(middleware.RequireAdmin(http.HandlerFunc(motori.PostTestEngine))))
	mux.Handle("POST /motori/{id}/wizard/attiva", authMW(middleware.RequireAdmin(http.HandlerFunc(motori.PostAttivaMotore))))
	mux.Handle("GET /motori/{id}", authMW(http.HandlerFunc(motori.GetDettaglio)))
	mux.Handle("POST /motori/{id}/duplica", authMW(middleware.RequireAdmin(http.HandlerFunc(motori.PostDuplica))))
	mux.Handle("POST /motori/{id}/archivia", authMW(middleware.RequireAdmin(http.HandlerFunc(motori.PostArchivia))))

	// Graduatorie
	mux.Handle("POST /motori/{id}/run", authMW(http.HandlerFunc(grad.PostCalcola)))
	mux.Handle("GET /motori/{id}/run/{runID}", authMW(http.HandlerFunc(grad.GetRun)))
	mux.Handle("GET /motori/{id}/run/{runID}/{anno}/{tipo}", authMW(http.HandlerFunc(grad.GetRunTabella)))
	mux.Handle("GET /motori/{id}/run/{runID}/export/{anno}/{tipo}", authMW(http.HandlerFunc(grad.GetExportCSV)))
	mux.Handle("POST /motori/{id}/run/{runID}/pubblica", authMW(middleware.RequireAdmin(http.HandlerFunc(grad.PostPubblica))))

	// Bulk actions
	mux.Handle("POST /motori/{id}/run/{runID}/approva-batch", authMW(http.HandlerFunc(acts.PostApprovaBatch)))
	mux.Handle("POST /motori/{id}/run/{runID}/rifiuta-batch", authMW(http.HandlerFunc(acts.PostRifiutaBatch)))

	// Audit
	mux.Handle("GET /audit", authMW(http.HandlerFunc(auditH.GetAudit)))

	return middleware.Recovery(middleware.SecurityHeaders(mux))
}
