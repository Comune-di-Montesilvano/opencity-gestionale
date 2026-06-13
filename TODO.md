# TODO — Gestionale OpenCity

## Bug noti

- [ ] `setup.html` mostra `{{.Op.JWT | printf "%s"}}` invece di `OPENCITY_BASE_URL` — passare BaseURL al template
- [ ] `go.mod`: `modernc.org/sqlite` e `github.com/google/uuid` marcati come `// indirect` — rendere diretti (`go get modernc.org/sqlite github.com/google/uuid`)
- [ ] `render.go`: cache template con `sync.Once` non si ricarica in sviluppo — aggiungere route `GET /dev/reload-templates` gated su flag build o env `DEV=true`
- [ ] `run_tabella.html`: colonna "Corrispettivo netto" mostra `Istanza.Corrispettivo` (lordo) — usare `CorrispettivoNetto` via FuncMap
- [ ] Sessioni scadute non vengono mai pulite — chiamare `db.PulisciSessioniScadute` all'avvio e/o ogni N ore (goroutine background in `cmd/server/main.go`)
- [ ] Handler `PutBando` usa `POST /bandi/{id}` — i browser non supportano `PUT` nativo via form; già corretto nel router ma verificare con HTMX `hx-method="PUT"` o aggiungere campo `_method`
- [ ] `bando_form.html` non ha action corretta per modifica (`/bandi/{id}` via PUT) — da testare end-to-end

---

## Completamento features

### Gestionale core

- [ ] **Modifica bando**: `bando_dettaglio.html` ha href `#` per il bottone Modifica — collegare a form
- [ ] **Paginazione runs**: `ListRuns` ritorna tutte le run senza paging — aggiungere `LIMIT/OFFSET` e paginazione UI per bandi con molte run
- [ ] **Flash messages**: redirect dopo POST (login, salva bando, calcola run) non porta feedback — aggiungere cookie flash o query param `?msg=`
- [ ] **Indicatore calcolo in corso**: `POST /bandi/{id}/run` può durare 30+ secondi (332 istanze) — mostrare spinner HTMX o risposta SSE con progress
- [ ] **Contatore "Ammesse pronte"** nella tabella run: distinguere pratiche già approvate su OpenCity da approvare — aggiungere colonna `stato_opencity` derivata da `Istanza.Status`

### Wizard setup

- [ ] **Step 2 form**: checkbox + campi per ogni servizio sono statici (tutti i servizi, non solo quelli selezionati) — filtrare con JS: mostrare campi solo per servizi con checkbox checked
- [ ] **Idempotenza**: `PostSetupStep2` inserisce sempre nuovi bandi — controllare se esiste già un bando con stesso `service_id` prima di inserire
- [ ] **Validazione**: budget e ISEE obbligatori lato server — ritornare errore se mancanti

### Bulk actions

- [ ] **Progress streaming**: `confirmBatch()` in `run_tabella.html` aspetta risposta JSON finale — per N > 50 pratiche mostrare progress via SSE o polling
- [ ] **Stato pratiche già processate**: checkbox visibili anche per pratiche già approvate — filtrare per `Istanza.Status != "accept"`
- [ ] **Messaggio precompilato**: textarea del modal vuota — precompilare con testo standard del bando (da campo `engine_config` JSON o costante per engine)

### Audit

- [ ] **Filtro per bando**: dropdown bandi nel filtro audit — `ListBandi` già disponibile, passare al template
- [ ] **Paginazione**: offset/limit passato via query param ma link "Precedente/Successiva" non porta tutti i filtri attivi — correggere URL pagination

---

## Qualità & robustezza

### Go fixes

- [ ] `go get modernc.org/sqlite` — rimuovere `// indirect`
- [ ] `go get github.com/google/uuid` — rimuovere `// indirect`
- [ ] Aggiungere `.gitignore`:
  ```
  /data/
  /output/
  *.db
  gestionale
  .env
  ```
- [ ] Aggiungere `.env.example`:
  ```
  OPENCITY_BASE_URL=https://service.comune.montesilvano.pe.it
  SECRET_KEY=<genera con: openssl rand -hex 32>
  ADMIN_USERNAMES=apioperator
  DB_PATH=./data/gestionale.db
  PORT=8080
  ```

### Error handling

- [ ] Template `404.html` e `500.html` — handler `func(w, r)` per `http.NotFound` e internal error
- [ ] Handler `/` (root redirect) ritorna 404 per path sconosciuti — aggiungere pagina 404 custom
- [ ] `PostCalcola`: errore durante `FetchAllApplications` con timeout 30s — aumentare timeout client per calcoli batch (ora 30s in `NewClient`)

### Sicurezza

- [ ] Cookie `Secure: true` in produzione (quando `PORT != 8080` o env `HTTPS=true`)
- [ ] Header `Content-Security-Policy`, `X-Frame-Options`, `X-Content-Type-Options` — aggiungere middleware
- [ ] Rate limiting su `POST /login` — prevenire brute force credenziali OpenCity

---

## Testing

- [ ] **Unit test `internal/graduatoria/engine_test.go`**: testare `CalcolaConConfig` con fixture JSON (snapshot di 5-10 istanze reali anonimizzate)
- [ ] **Unit test `internal/graduatoria/checks_test.go`**: `BonusNidiCoerente`, `IseeScaduto`, `IseeDaVerificare`
- [ ] **Unit test `internal/db/`**: test con DB in-memory (`:memory:`) per CRUD bandi/runs/audit/sessioni
- [ ] **Integration test handler login**: mock OpenCity con `httptest.NewServer` che ritorna JWT fissato
- [ ] **Test CLI batch**: `go run ./cmd/batch` con env mockato — verificare che output CSV = risultati giugno 2026 documentati in CLAUDE.md

---

## Futuri engine (multi-servizio)

- [ ] **Libri di testo** (`aeffaacf-adad-461b-83f0-ee3d95d87f31`): 629 istanze — creare `internal/graduatoria/libri/engine.go`
- [ ] **Centri estivi** (`05a37702-0710-43eb-8165-3a11fc766f49`): 161 istanze — creare `internal/graduatoria/centri_estivi/engine.go`
- [ ] **Rimborso viaggio riabilitazione** (`10987e1d-afa3-4b53-83fb-ef2c2db04cdb`): 7 istanze
- [ ] Ogni nuovo engine: implementare `ServiceEngine` interface (`Name`, `Calcola`, `CSVHeaders`, `CSVRecord`), registrare via `init()`, aggiungere opzione nel dropdown `bando_form.html`

---

## Deploy

- [ ] **GitHub repo**: inizializzare git, primo commit, push
- [ ] **GHCR**: verificare build Docker su GitHub Actions (immagine `ghcr.io/<owner>/opencity-backend`)
- [ ] **Proxmox Podman**: `podman run --rm -v /opt/gestionale/data:/data:z ...` — script systemd unit
- [ ] **Reverse proxy**: nginx/Caddy davanti per TLS — header `X-Forwarded-Proto` per cookie `Secure`
- [ ] **Backup SQLite**: cron `sqlite3 /data/gestionale.db ".backup /backup/gestionale-$(date +%Y%m%d).db"`
- [ ] **Health check**: aggiungere `GET /health` → `200 OK` (usato da Docker e load balancer)

---

## Stato attuale (giugno 2026)

Implementato e compilante:
- ✅ CLI batch: fetch → calcola → CSV + HTML prospetto operatori
- ✅ `internal/graduatoria`: engine mense_rette, ServiceEngine interface, checks, CSV helpers
- ✅ `internal/opencity/client.go`: Login/NewClient/Approve/Reject/GetUser/FetchServices
- ✅ `internal/db`: SQLite WAL, schema embedded, CRUD bandi/runs/audit/sessioni
- ✅ `internal/config`: env vars con validazione
- ✅ Web server: router Go 1.22+, middleware auth, tutti gli handler
- ✅ Template HTML: login, dashboard, bandi, run tabella, audit, setup wizard
- ✅ Static: htmx.min.js bundled, style.css
- ✅ Dockerfile multi-stage distroless + docker-compose + GitHub Actions

Non ancora testato end-to-end (nessuna sessione reale avviata contro OpenCity live).
