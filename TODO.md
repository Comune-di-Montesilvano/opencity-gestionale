# TODO — Gestionale OpenCity

---

## Feature pending

### Bulk actions

- [ ] **Progress streaming**: `confirmBatch()` in `run_tabella.html` aspetta risposta JSON finale — per N > 50 pratiche aggiungere polling HTMX o SSE
- [ ] **Stato pratiche già processate**: checkbox visibili anche per pratiche già approvate — filtrare per `Istanza.Status != "accept"` (campo già presente in struct)

### Gestionale

- [ ] **Paginazione runs**: `ListRuns` ritorna tutte le run senza paging — aggiungere `LIMIT/OFFSET` + paginazione UI per bandi con molte run
- [ ] **Wizard mapping step 3**: mostrare campo `PDNDPath` per ogni campo mappato (path della firma PDND, es. `ordinary_economic_situation_indicator.meta.signature`)
- [ ] **Istruttoria — link pratica OpenCity**: nella dashboard istruttoria, aggiungere link diretto alla pratica su OpenCity per ogni riga

---

## Sicurezza

- [x] Rate limiting su `POST /login` — implementato, max 5 tentativi/IP in 15 min → HTTP 429

---

## Deploy

- [ ] **Proxmox Podman**: systemd unit con `podman run --rm -v /opt/gestionale/data:/data:z ghcr.io/comune-di-montesilvano/opencity-gestionale:latest`
- [ ] **Reverse proxy TLS**: nginx/Caddy davanti per HTTPS — impostare `TRUST_PROXY=true` per cookie Secure
- [ ] **Backup SQLite**: cron `sqlite3 /data/gestionale.db ".backup /backup/gestionale-$(date +%Y%m%d).db"`

---

## Testing

- [ ] Integration test handler login: mock OpenCity con `httptest.NewServer` che ritorna JWT fisso
- [ ] Test istruttoria: unit test `FlagMotivi` con record PDND/non-PDND e filtri_flag custom

---

## Stato (giugno 2026)

✅ Completato e compilante:

- `internal/graduatoria/cf`: helper puri CF italiano — EtaAnni, AnnoBirth, Sesso, ComuneNascita, Valido (checksum Agenzia Entrate)
- `internal/graduatoria`: engine `generic` universale con `EngineConfig.Modalita` (fondi/posti/ammissione/lista_attesa), 26+ operatori PassaFiltro (numerico/stringa/booleano/data/CF), VerificaConfig, PDNDPath in FieldMapping
- `internal/graduatoria/generic`: dispatch per modalità, buildGraduatoria* per ogni tipo, EstraiRecords esportato
- `internal/db`: tabella `istruttorie` con UpsertIstruttoria, BatchSetStato, CountPending, ListEscluse, GetIstruttoriaStats
- `internal/opencity/client.go`: Login/NewClient/Approve/Reject/GetUser/FetchServices/FetchAllApplications
- `internal/config`: env vars con validazione
- Web server: router Go 1.22+, middleware auth/admin/rate-limit/recovery/security-headers
- Motori wizard (7 step adattivi): connessione servizio → tipo bando → mapping campi → filtri + istruttoria → tipologie (skip ammissione/lista_attesa) → rimborso (solo fondi) → test+attiva
- Istruttoria pre-calcolo: `/motori/{id}/istruttoria` dashboard, scansiona app, approva/escludi batch, calcolo bloccato se pending
- Workflow pubblicazione run: bozza → pubblicata (solo admin)
- Documento stampabile: `/stampa` con colonne configurabili, CF oscurato, `@media print` CSS
- Dashboard: mostra solo motori attivi, ultima run per operatore
- Audit trail: insert su ogni azione (calcola/approva/rifiuta/pubblica/istruttoria_*), view con filtri
- Bulk approve/reject: HTMX modal, JSON response, audit logging
- Template HTML: 25+ template con pattern `base.html` + block
- Dockerfile multi-stage distroless + docker-compose + GitHub Actions CI/CD (test → build → GHCR → release)
- Test: 66 subtests PassaFiltro, 5 test engine (4 modalità + dedup), test CF, test DB CRUD
