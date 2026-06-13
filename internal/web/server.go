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
	setup := &handlers.SetupHandler{
		DB:      dbConn,
		BaseURL: cfg.OpenCityBaseURL,
	}
	dashboard := &handlers.DashboardHandler{DB: dbConn}
	bandi := &handlers.BandiHandler{DB: dbConn}
	grad := &handlers.GraduatoriaHandler{DB: dbConn, BaseURL: cfg.OpenCityBaseURL}
	acts := &handlers.ActionsHandler{DB: dbConn, BaseURL: cfg.OpenCityBaseURL}
	auditH := &handlers.AuditHandler{DB: dbConn}

	authMW := middleware.Auth(dbConn)

	mux := http.NewServeMux()

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

	// Setup wizard (admin only)
	mux.Handle("GET /setup", authMW(middleware.RequireAdmin(http.HandlerFunc(setup.GetSetup))))
	mux.Handle("POST /setup/step/1", authMW(middleware.RequireAdmin(http.HandlerFunc(setup.PostSetupStep1))))
	mux.Handle("POST /setup/step/2", authMW(middleware.RequireAdmin(http.HandlerFunc(setup.PostSetupStep2))))

	// Bandi
	mux.Handle("GET /bandi", authMW(http.HandlerFunc(bandi.ListBandi)))
	mux.Handle("GET /bandi/nuovo", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.GetNuovoBando))))
	mux.Handle("POST /bandi", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostBando))))
	mux.Handle("GET /bandi/{id}", authMW(http.HandlerFunc(bandi.GetBando)))
	mux.Handle("POST /bandi/{id}", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PutBando))))
	mux.Handle("POST /bandi/{id}/disattiva", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.DeleteBando))))

	// Graduatorie
	mux.Handle("POST /bandi/{id}/run", authMW(http.HandlerFunc(grad.PostCalcola)))
	mux.Handle("GET /bandi/{id}/run/{runID}", authMW(http.HandlerFunc(grad.GetRun)))
	mux.Handle("GET /bandi/{id}/run/{runID}/{anno}/{tipo}", authMW(http.HandlerFunc(grad.GetRunTabella)))
	mux.Handle("GET /bandi/{id}/run/{runID}/export/{anno}/{tipo}", authMW(http.HandlerFunc(grad.GetExportCSV)))

	// Bulk actions
	mux.Handle("POST /bandi/{id}/run/{runID}/approva-batch", authMW(http.HandlerFunc(acts.PostApprovaBatch)))
	mux.Handle("POST /bandi/{id}/run/{runID}/rifiuta-batch", authMW(http.HandlerFunc(acts.PostRifiutaBatch)))

	// Audit
	mux.Handle("GET /audit", authMW(http.HandlerFunc(auditH.GetAudit)))

	return mux
}
