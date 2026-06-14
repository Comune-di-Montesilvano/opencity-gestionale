# TODO — Gestionale OpenCity

---

## Feature pending

### Bulk actions

- [ ] **Progress streaming**: `confirmBatch()` in `run_tabella.html` aspetta risposta JSON finale — per N > 50 pratiche aggiungere polling HTMX o SSE
- [ ] **Stato pratiche già processate**: checkbox visibili anche per pratiche già approvate — filtrare per `Istanza.Status != "accept"` (campo già presente in struct)

### Gestionale

- [ ] **Paginazione runs**: `ListRuns` ritorna tutte le run senza paging — aggiungere `LIMIT/OFFSET` + paginazione UI per bandi con molte run

---

## Sicurezza

- [ ] Rate limiting su `POST /login` — prevenire brute force credenziali OpenCity

---

## Deploy

- [ ] **Proxmox Podman**: systemd unit con `podman run --rm -v /opt/gestionale/data:/data:z ghcr.io/comune-di-montesilvano/opencity-gestionale:latest`
- [ ] **Reverse proxy TLS**: nginx/Caddy davanti per HTTPS — impostare `TRUST_PROXY=true` per cookie Secure
- [ ] **Backup SQLite**: cron `sqlite3 /data/gestionale.db ".backup /backup/gestionale-$(date +%Y%m%d).db"`

---

## Testing

- [ ] Unit test `internal/graduatoria/engine_test.go`: `CalcolaConConfig` con fixture JSON (5-10 istanze anonimizzate)
- [ ] Unit test `internal/graduatoria/checks_test.go`: `BonusNidiCoerente`, `IseeScaduto`, `IseeDaVerificare`
- [ ] Integration test handler login: mock OpenCity con `httptest.NewServer` che ritorna JWT fisso

---

## Futuri engine (multi-servizio)

- [ ] **Libri di testo** (`aeffaacf-adad-461b-83f0-ee3d95d87f31`, 629 istanze)
- [ ] **Centri estivi** (`05a37702-0710-43eb-8165-3a11fc766f49`, 161 istanze)
- [ ] **Rimborso viaggio riabilitazione** (`10987e1d-afa3-4b53-83fb-ef2c2db04cdb`, 7 istanze)

Per ogni engine: implementare `ServiceEngine` interface, registrare via `init()` in entrambi i binari.

---

## Stato (giugno 2026)

✅ Completato e compilante:

- CLI batch: fetch → calcola → CSV + HTML prospetto operatori
- `internal/graduatoria`: engine `mense_rette` legacy, engine `generic` configurabile, ServiceEngine interface, checks, CSV helpers
- `internal/opencity/client.go`: Login/NewClient/Approve/Reject/GetUser/FetchServices
- `internal/db`: SQLite WAL, schema embedded, CRUD bandi/runs/audit/sessioni, pulizia sessioni scadute
- `internal/config`: env vars con validazione
- Web server: router Go 1.22+, middleware auth/admin/recovery/security-headers
- Motori wizard (6 step): connessione servizio → mapping campi → filtri → tipologie/ordinamento/dedup → rimborso → test+attiva
- Engine generico: `EngineConfig` JSON, filtri, deduplicazione, espansione per-anno, mapping dot-notation
- Workflow pubblicazione run: bozza → pubblicata (solo admin), operatori vedono solo pubblicate
- Documento stampabile: `/stampa` con colonne configurabili, CF oscurato, `@media print` CSS
- Dashboard: mostra solo motori attivi, ultima run per operatore
- Audit trail: insert su ogni azione, view con filtri operatore/azione/bando
- Bulk approve/reject: HTMX modal, JSON response, audit logging
- Template HTML: 20+ template con pattern `base.html` + block
- Dockerfile multi-stage distroless + docker-compose + GitHub Actions CI/CD (test → build → GHCR → release)
- `.env` locale, `.env.example`, `.gitignore`
