package handlers_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"opencity-gestionale/internal/db"
	"opencity-gestionale/internal/web/handlers"
	"opencity-gestionale/internal/web/middleware"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open :memory:: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestPostExportColonne(t *testing.T) {
	d := openTestDB(t)
	id, err := db.InsertBando(d, &db.Bando{
		ServiceID:  "test",
		Nome:       "Test",
		EngineType: "generic",
		EngineConfig: "{}",
		StatoBando: "attivo",
		Attivo:     true,
		CreatedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("InsertBando: %v", err)
	}

	h := &handlers.BandiHandler{DB: d}

	form := url.Values{"col": []string{"isee", "figlio_cf"}}
	req := httptest.NewRequest("POST", "/bandi/"+strconv.FormatInt(id, 10)+"/export-colonne",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(middleware.WithOperator(req.Context(), &middleware.OperatorCtx{Ruolo: "admin"}))
	req.SetPathValue("id", strconv.FormatInt(id, 10))

	w := httptest.NewRecorder()
	h.PostExportColonne(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}
	b, _ := db.GetBando(d, id)
	var cols []string
	json.Unmarshal([]byte(b.ExportColonne), &cols)
	if len(cols) != 2 || cols[0] != "isee" {
		t.Fatalf("ExportColonne non salvate correttamente: %v", cols)
	}
}
