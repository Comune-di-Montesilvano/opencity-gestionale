package web

import (
	"database/sql"
	"net/http"

	"opencity-gestionale/internal/config"
	"opencity-gestionale/internal/opencity"
	"opencity-gestionale/internal/web/handlers"
	"opencity-gestionale/internal/web/middleware"
)

func NewServer(cfg *config.Config, dbConn *sql.DB, branding *opencity.Branding, appVersion string) http.Handler {
	handlers.SetBranding(branding)
	handlers.SetVersion(appVersion)

	auth := &handlers.AuthHandler{
		DB:             dbConn,
		BaseURL:        cfg.OpenCityBaseURL,
		AdminUsernames: cfg.AdminUsernames,
		SecureCookie:   cfg.TrustProxy,
	}
	dashboard := &handlers.DashboardHandler{DB: dbConn}
	bandi := &handlers.BandiHandler{DB: dbConn, BaseURL: cfg.OpenCityBaseURL}
	grad := &handlers.GraduatoriaHandler{DB: dbConn, BaseURL: cfg.OpenCityBaseURL}
	acts := &handlers.ActionsHandler{DB: dbConn, BaseURL: cfg.OpenCityBaseURL}
	auditH := &handlers.AuditHandler{DB: dbConn}
	istr := &handlers.IstruttoriaHandler{DB: dbConn, BaseURL: cfg.OpenCityBaseURL}
	expMap := &handlers.ExportMappingsHandler{DB: dbConn, BaseURL: cfg.OpenCityBaseURL}

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
	mux.Handle("POST /login", middleware.LoginRateLimit(http.HandlerFunc(auth.PostLogin)))
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

	// Bandi — lista e wizard (admin only per modifica)
	mux.Handle("GET /bandi", authMW(http.HandlerFunc(bandi.GetLista)))
	mux.Handle("GET /bandi/nuovo", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.GetNuovo))))
	mux.Handle("POST /bandi/wizard/connetti", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostConnettiServizi))))
	mux.Handle("POST /bandi/wizard/crea", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostCreaBando))))
	mux.Handle("GET /bandi/{id}/wizard/{step}", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.GetWizardStep))))
	mux.Handle("POST /bandi/{id}/wizard/{step}", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostWizardStep))))
	mux.Handle("POST /bandi/{id}/wizard/test", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostTestEngine))))
	mux.Handle("POST /bandi/{id}/wizard/attiva", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostAttivaBando))))
	mux.Handle("GET /bandi/{id}/api/valori-campo", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.GetValoriCampo))))
	mux.Handle("GET /bandi/{id}/api/statistiche-campo", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.GetStatisticheField))))
	mux.Handle("POST /bandi/{id}/wizard/superset", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostBuildSuperset))))
	mux.Handle("GET /bandi/{id}", authMW(http.HandlerFunc(bandi.GetDettaglio)))
	mux.Handle("POST /bandi/{id}/duplica", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostDuplica))))
	mux.Handle("POST /bandi/{id}/archivia", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostArchivia))))
	mux.Handle("GET /bandi/{id}/export", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.GetExportBando))))
	mux.Handle("GET /bandi/import", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.GetImport))))
	mux.Handle("POST /bandi/import", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostImportBando))))
	mux.Handle("POST /bandi/{id}/rinomina", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostRinomina))))
	mux.Handle("GET /bandi/{id}/parametri/edit", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.GetEditParametri))))
	mux.Handle("POST /bandi/{id}/parametri/edit", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostEditParametri))))
	mux.Handle("POST /bandi/{id}/export-colonne", authMW(middleware.RequireAdmin(http.HandlerFunc(bandi.PostExportColonne))))

	// Graduatorie
	mux.Handle("POST /bandi/{id}/run", authMW(http.HandlerFunc(grad.PostCalcola)))
	mux.Handle("GET /bandi/{id}/run/{runID}", authMW(http.HandlerFunc(grad.GetRun)))
	mux.Handle("GET /bandi/{id}/run/{runID}/{anno}/{tipo}", authMW(http.HandlerFunc(grad.GetRunTabella)))
	mux.Handle("GET /bandi/{id}/run/{runID}/export/{anno}/{tipo}", authMW(http.HandlerFunc(grad.GetExportCSV)))
	mux.Handle("GET /bandi/{id}/run/{runID}/iban", authMW(http.HandlerFunc(grad.GetIBANPage)))
	mux.Handle("POST /bandi/{id}/iban/config", authMW(http.HandlerFunc(bandi.PostSaveIBANConfig)))
	mux.Handle("GET /bandi/{id}/run/{runID}/export/iban", authMW(http.HandlerFunc(grad.GetExportIBAN)))
	mux.Handle("POST /bandi/{id}/run/{runID}/pubblica", authMW(middleware.RequireAdmin(http.HandlerFunc(grad.PostPubblica))))
	mux.Handle("POST /bandi/{id}/run/{runID}/elimina", authMW(middleware.RequireAdmin(http.HandlerFunc(grad.PostEliminaRun))))
	mux.Handle("GET /bandi/{id}/run/{runID}/stampa", authMW(http.HandlerFunc(grad.GetStampa)))
	mux.Handle("GET /bandi/{id}/run/{runID}/gruppo/{nome}", authMW(http.HandlerFunc(grad.GetRunGruppo)))
	mux.Handle("GET /bandi/{id}/run/{runID}/export/gruppo/{nome}", authMW(http.HandlerFunc(grad.GetExportCSVGruppo)))

	// Istruttoria pre-calcolo
	mux.Handle("GET /bandi/{id}/istruttoria", authMW(http.HandlerFunc(istr.GetIstruttoria)))
	mux.Handle("GET /bandi/{id}/dati", authMW(http.HandlerFunc(istr.GetDatiLocali)))
	mux.Handle("POST /bandi/{id}/istruttoria/scansiona", authMW(http.HandlerFunc(istr.PostScansiona)))
	mux.Handle("POST /bandi/{id}/istruttoria/batch", authMW(http.HandlerFunc(istr.PostIstruttoriaBatch)))
	mux.Handle("POST /bandi/{id}/istruttoria/{praticaID}/dato", authMW(http.HandlerFunc(istr.PostSaveDato)))
	mux.Handle("POST /bandi/{id}/istruttoria/{praticaID}/nota", authMW(http.HandlerFunc(istr.PostSaveNota)))
	mux.Handle("POST /bandi/{id}/istruttoria/{praticaID}/salva-tutto", authMW(http.HandlerFunc(istr.PostSalvaTutto)))
	mux.Handle("POST /bandi/{id}/istruttoria/{praticaID}/riapri", authMW(http.HandlerFunc(istr.PostRiapri)))
	mux.Handle("POST /bandi/{id}/istruttoria/{praticaID}/stato", authMW(http.HandlerFunc(istr.PostEscludiDati)))
	mux.Handle("POST /bandi/{id}/istruttoria/{praticaID}/includi-dufficio", authMW(http.HandlerFunc(istr.PostToggleIncludiDufficio)))

	// Bulk actions
	mux.Handle("POST /bandi/{id}/run/{runID}/approva-batch", authMW(http.HandlerFunc(acts.PostApprovaBatch)))
	mux.Handle("POST /bandi/{id}/run/{runID}/rifiuta-batch", authMW(http.HandlerFunc(acts.PostRifiutaBatch)))

	// Export mappings (template CSV configurabili per bando)
	mux.Handle("GET /bandi/{id}/export-mappings", authMW(middleware.RequireAdmin(http.HandlerFunc(expMap.GetExportMappings))))
	mux.Handle("POST /bandi/{id}/export-mappings", authMW(middleware.RequireAdmin(http.HandlerFunc(expMap.PostCreateMapping))))
	mux.Handle("GET /bandi/{id}/export-mappings/{mapID}", authMW(middleware.RequireAdmin(http.HandlerFunc(expMap.GetEditMapping))))
	mux.Handle("POST /bandi/{id}/export-mappings/{mapID}", authMW(middleware.RequireAdmin(http.HandlerFunc(expMap.PostSaveMapping))))
	mux.Handle("POST /bandi/{id}/export-mappings/{mapID}/delete", authMW(middleware.RequireAdmin(http.HandlerFunc(expMap.PostDeleteMapping))))
	mux.Handle("GET /bandi/{id}/run/{runID}/export/mapping/{mapID}", authMW(http.HandlerFunc(expMap.GetExportCSVMapped)))

	// Audit
	mux.Handle("GET /audit", authMW(http.HandlerFunc(auditH.GetAudit)))

	// Dev only
	if cfg.DevMode {
		mux.HandleFunc("GET /dev/reload-templates", func(w http.ResponseWriter, r *http.Request) {
			handlers.ReloadTemplates()
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("template ricaricati"))
		})
	}

	return middleware.Recovery(middleware.SecurityHeaders(mux))
}
