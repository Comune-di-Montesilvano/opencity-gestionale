package db_test

import (
	"database/sql"
	"testing"
	"time"

	"opencity-gestionale/internal/db"
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

// --- Bandi ---

func TestBandiCRUD(t *testing.T) {
	conn := openTestDB(t)

	b := &db.Bando{
		ServiceID:             "svc-1",
		Nome:                  "Test Bando",
		BudgetTotale:          71096.37,
		ISEEMassimo:           40000,
		ScadenzaPresentazione: "2026-04-24",
		EngineType:            "mense_rette",
		EngineConfig:          "{}",
		Attivo:                true,
		CreatedAt:             time.Now(),
	}

	id, err := db.InsertBando(conn, b)
	if err != nil {
		t.Fatalf("InsertBando: %v", err)
	}
	if id == 0 {
		t.Fatal("id deve essere > 0")
	}

	got, err := db.GetBando(conn, id)
	if err != nil {
		t.Fatalf("GetBando: %v", err)
	}
	if got.Nome != b.Nome {
		t.Errorf("nome: got %q, want %q", got.Nome, b.Nome)
	}
	if got.BudgetTotale != b.BudgetTotale {
		t.Errorf("budget: got %f, want %f", got.BudgetTotale, b.BudgetTotale)
	}
	if !got.Attivo {
		t.Error("deve essere attivo")
	}

	got.Nome = "Aggiornato"
	if err := db.UpdateBando(conn, got); err != nil {
		t.Fatalf("UpdateBando: %v", err)
	}
	got2, _ := db.GetBando(conn, id)
	if got2.Nome != "Aggiornato" {
		t.Errorf("nome post-update: got %q", got2.Nome)
	}

	if err := db.DisattivaBando(conn, id); err != nil {
		t.Fatalf("DisattivaBando: %v", err)
	}
	got3, _ := db.GetBando(conn, id)
	if got3.Attivo {
		t.Error("deve essere inattivo dopo DisattivaBando")
	}

	n, _ := db.CountBandi(conn)
	if n != 1 {
		t.Errorf("CountBandi: got %d, want 1", n)
	}
}

func TestGetBandoByServiceID(t *testing.T) {
	conn := openTestDB(t)

	b := &db.Bando{ServiceID: "svc-abc", Nome: "ABC", BudgetTotale: 1000, EngineType: "mense_rette", EngineConfig: "{}", Attivo: true, CreatedAt: time.Now()}
	db.InsertBando(conn, b)

	got, err := db.GetBandoByServiceID(conn, "svc-abc")
	if err != nil {
		t.Fatalf("GetBandoByServiceID: %v", err)
	}
	if got.Nome != "ABC" {
		t.Errorf("got %q, want %q", got.Nome, "ABC")
	}

	_, err = db.GetBandoByServiceID(conn, "inesistente")
	if err == nil {
		t.Error("atteso errore per service_id inesistente")
	}
}

func TestListBandi(t *testing.T) {
	conn := openTestDB(t)

	for i, nome := range []string{"A", "B", "C"} {
		db.InsertBando(conn, &db.Bando{
			ServiceID: "svc-" + nome, Nome: nome, EngineType: "mense_rette",
			EngineConfig: "{}", Attivo: i%2 == 0, CreatedAt: time.Now(),
		})
	}

	bandi, err := db.ListBandi(conn)
	if err != nil {
		t.Fatalf("ListBandi: %v", err)
	}
	if len(bandi) != 3 {
		t.Errorf("got %d bandi, want 3", len(bandi))
	}
}

func TestCountBandiPerStato(t *testing.T) {
	conn := openTestDB(t)

	inserisci := func(stato string) {
		b := &db.Bando{
			ServiceID: "svc-" + stato + "-" + time.Now().String(),
			Nome:      "Bando " + stato,
			EngineType:   "generic",
			EngineConfig: "{}",
			Attivo:       stato != "archiviato",
			StatoMotore:  stato,
			CreatedAt:    time.Now(),
		}
		db.InsertBando(conn, b)
	}

	inserisci("attivo")
	inserisci("attivo")
	inserisci("bozza")
	inserisci("archiviato")

	counts, err := db.CountBandiPerStato(conn)
	if err != nil {
		t.Fatalf("CountBandiPerStato: %v", err)
	}
	if counts["attivo"] != 2 {
		t.Errorf("attivo: got %d, want 2", counts["attivo"])
	}
	if counts["bozza"] != 1 {
		t.Errorf("bozza: got %d, want 1", counts["bozza"])
	}
	if counts["archiviato"] != 1 {
		t.Errorf("archiviato: got %d, want 1", counts["archiviato"])
	}
}

// --- Sessioni ---

func TestSessioniCRUD(t *testing.T) {
	conn := openTestDB(t)

	s := &db.Sessione{
		ID:          "sess-1",
		Operatore:   "mario",
		UserID:      "u1",
		JWTOpenCity: "jwt-token",
		Ruolo:       "admin",
		ServiceIDs:  `["svc-1"]`,
		ScadeAt:     time.Now().Add(10 * 24 * time.Hour),
		CreatedAt:   time.Now(),
	}

	if err := db.InsertSessione(conn, s); err != nil {
		t.Fatalf("InsertSessione: %v", err)
	}

	got, err := db.GetSessione(conn, "sess-1")
	if err != nil {
		t.Fatalf("GetSessione: %v", err)
	}
	if got == nil {
		t.Fatal("sessione nil")
	}
	if got.Operatore != "mario" {
		t.Errorf("operatore: got %q, want %q", got.Operatore, "mario")
	}
	if got.Ruolo != "admin" {
		t.Errorf("ruolo: got %q, want %q", got.Ruolo, "admin")
	}

	if err := db.DeleteSessione(conn, "sess-1"); err != nil {
		t.Fatalf("DeleteSessione: %v", err)
	}
	got2, _ := db.GetSessione(conn, "sess-1")
	if got2 != nil {
		t.Error("sessione deve essere nil dopo delete")
	}
}

func TestPulisciSessioniScadute(t *testing.T) {
	conn := openTestDB(t)

	db.InsertSessione(conn, &db.Sessione{
		ID: "scaduta", Operatore: "u1", UserID: "1", JWTOpenCity: "j", Ruolo: "operator",
		ServiceIDs: "[]", ScadeAt: time.Now().Add(-1 * time.Hour), CreatedAt: time.Now(),
	})
	db.InsertSessione(conn, &db.Sessione{
		ID: "valida", Operatore: "u2", UserID: "2", JWTOpenCity: "j", Ruolo: "operator",
		ServiceIDs: "[]", ScadeAt: time.Now().Add(24 * time.Hour), CreatedAt: time.Now(),
	})

	if err := db.PulisciSessioniScadute(conn); err != nil {
		t.Fatalf("PulisciSessioniScadute: %v", err)
	}

	scaduta, _ := db.GetSessione(conn, "scaduta")
	if scaduta != nil {
		t.Error("sessione scaduta deve essere rimossa")
	}
	valida, _ := db.GetSessione(conn, "valida")
	if valida == nil {
		t.Error("sessione valida non deve essere rimossa")
	}
}

// --- Runs ---

func TestRunsCRUD(t *testing.T) {
	conn := openTestDB(t)

	b := &db.Bando{ServiceID: "svc-1", Nome: "Bando", EngineType: "mense_rette", EngineConfig: "{}", Attivo: true, CreatedAt: time.Now()}
	bandoID, _ := db.InsertBando(conn, b)

	run := &db.GraduatoriaRun{
		BandoID:     bandoID,
		CalcolataDa: "mario",
		CalcolataAt: time.Now(),
		DatiJSON:    `{"Gruppi":[],"Escluse":null}`,
		NumTotale:   100,
		NumAmmesse:  80,
		NumEscluse:  20,
		BudgetUsato: 35000,
	}

	runID, err := db.InsertRun(conn, run)
	if err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	if runID == 0 {
		t.Fatal("runID deve essere > 0")
	}

	runs, err := db.ListRuns(conn, bandoID)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].NumAmmesse != 80 {
		t.Errorf("NumAmmesse: got %d, want 80", runs[0].NumAmmesse)
	}
}

// --- Audit ---

func TestAuditCRUD(t *testing.T) {
	conn := openTestDB(t)

	b := &db.Bando{ServiceID: "svc-1", Nome: "B", EngineType: "mense_rette", EngineConfig: "{}", Attivo: true, CreatedAt: time.Now()}
	bandoID, _ := db.InsertBando(conn, b)

	for _, azione := range []string{"calcola", "approva", "rifiuta"} {
		db.InsertAudit(conn, &db.AuditAction{
			Operatore: "mario",
			Azione:    azione,
			BandoID:   bandoID,
			Esito:     "ok",
		})
	}

	actions, total, err := db.ListAudit(conn, db.AuditFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if total != 3 {
		t.Errorf("total: got %d, want 3", total)
	}
	if len(actions) != 3 {
		t.Errorf("actions: got %d, want 3", len(actions))
	}

	filtered, tot2, _ := db.ListAudit(conn, db.AuditFilter{Azione: "approva", Limit: 50})
	if tot2 != 1 {
		t.Errorf("filtro azione: got %d, want 1", tot2)
	}
	if filtered[0].Azione != "approva" {
		t.Errorf("azione filtrata: got %q", filtered[0].Azione)
	}

	byBando, tot3, _ := db.ListAudit(conn, db.AuditFilter{BandoID: bandoID, Limit: 50})
	if tot3 != 3 {
		t.Errorf("filtro bando: got %d, want 3", tot3)
	}
	_ = byBando
}
