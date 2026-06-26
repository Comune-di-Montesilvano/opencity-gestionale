# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Progetto

Gestionale web per il Comune di Montesilvano: fetch istanze da OpenCity Italia (La Stanza del Cittadino), calcolo graduatorie FSE+, bulk approve/reject con audit trail. Il CSV nativo di OpenCity tronca campi Form.IO annidati — da qui la necessità di chiamare l'API con `version=2`.

Un solo binario: `cmd/server` (web gestionale). Il CLI batch `cmd/batch` è stato rimosso — tutte le funzionalità sono nel web server.

## Comandi

```bash
go build ./...
go test ./...
go vet ./...

# Web server locale
DB_PATH=./dev.db \
OPENCITY_BASE_URL=https://service.comune.montesilvano.pe.it \
SECRET_KEY=devdevdevdevdevdevdevdevdevdevdev \
ADMIN_USERNAMES=apioperator \
go run ./cmd/server

# Singolo test package
go test ./internal/graduatoria/...

docker compose up
```

### Env vars server (tutte in `internal/config/config.go`)

| Var | Default | Obbligatorio |
|-----|---------|--------------|
| `OPENCITY_BASE_URL` | — | sì |
| `SECRET_KEY` | — | sì, min 32 char |
| `ADMIN_USERNAMES` | — | CSV username, es. `apioperator` |
| `DB_PATH` | `gestionale.db` | no |
| `PORT` | `8080` | no |
| `DEV` | `false` | no — abilita `GET /dev/reload-templates` |

## Architettura non ovvia

### Engine unico: `generic`

Unico engine registrato: `generic` in `internal/graduatoria/generic/`. Si auto-registra via `func init()`. `cmd/server/main.go` importa:
```go
import _ "opencity-gestionale/internal/graduatoria/generic"
```

`ServiceEngine` interface in `internal/graduatoria/service.go`. Per aggiungere un engine: crea `internal/graduatoria/<nome>/engine.go`, implementa `ServiceEngine`, aggiungi blank import in `cmd/server/main.go`.

### Engine generico (`internal/graduatoria/generic/`)

Supporta qualsiasi bando tramite configurazione JSON in `bandi.engine_config`. La struttura `EngineConfig` è in `internal/graduatoria/config.go`.

**Struttura `EngineConfig`**:
```json
{
  "modalita": "fondi",
  "mapping": {
    "isee":   { "path": "ordinary_economic_situation_indicator.isee", "tipo": "float" },
    "tipo":   { "path": "tiporichiesta", "expand": true },
    "figlio_cf": { "path": "select_child", "tipo": "string" },
    "data_presentazione": { "path": "$app:submitted_at", "tipo": "time" }
  },
  "espansione": "anni",
  "filtri": [
    { "campo": "isee", "op": "<=", "valore": 40000 },
    { "campo": "figlio_cf", "op": "cf_eta_max", "valore": 14 }
  ],
  "deduplicazione": { "attiva": true, "chiave": ["figlio_cf", "annualita", "tipo"] },
  "ordinamento": [{ "campo": "isee", "dir": "asc" }],
  "tipologie": [
    { "nome": "rette", "campo": "tipo", "valore": "rette", "priorita": 1, "limite": { "tipo": "residuo" } }
  ],
  "rimborso": { "tipo": "netto", "campo_lordo": "corrispettivo", "campo_deduzione": "beneficio" }
}
```

**`Modalita`** determina il comportamento dell'engine:
- `"fondi"` — rimborso € per istanza, fino a esaurimento budget (`LimiteConfig.Tipo`: `"residuo"` | `"percentuale"` | `"fisso"`)
- `"posti"` — primi N ammessi, no rimborso (`LimiteConfig.Tipo`: `"posti"` | `"nessuno"`)
- `"ammissione"` — tutti che passano i filtri sono ammessi, no tipologie/rimborso
- `"lista_attesa"` — come ammissione ma ordinato per criterio

**Sintassi path**:
- `"foo.bar.baz"` — navigazione oggetto annidato nel payload `data`
- `"$app:submitted_at"` — campo top-level di `Application` (id, submitted_at, protocol_number, status)
- `"tipo": "count"` — lunghezza array
- `"expand": true` — campo relativo a ogni elemento dell'array `espansione`

**Campi derivati**: `corrispettivo_netto = corrispettivo - beneficio` calcolato automaticamente se `rimborso.tipo == "netto"`.

Il package `internal/graduatoria/extractor/` espone `Float`, `Str`, `Count`, `Time`, `ArrayElements`, `AppField` per navigazione dot-notation su `json.RawMessage`.

Il tipo `Record` (`internal/graduatoria/record.go`) raccoglie i campi estratti (FloatMap, StringMap, IntMap, TimeMap) e si converte in `*Istanza` via `ToIstanza()`.

### Filtri — operatori supportati (`PassaFiltro` in `record.go`)

Il campo `FiltroConfig.Op` supporta 26+ operatori, selezionati per dominio:

| Dominio | Operatori |
|---------|-----------|
| Numerico/Int | `<=` `>=` `==` `!=` `<` `>` `tra` (valore="min,max") |
| Stringa | `==` `!=` `contiene` `inizia_con` `finisce_con` `in` (lista a,b,c) `non_in` `vuoto` `non_vuoto` |
| Booleano | `vero` `falso` |
| Data | `prima_di` `dopo_di` `eta_max` `eta_min` (anni da data) |
| Codice Fiscale | `cf_eta_max` `cf_eta_min` `cf_anno_max` `cf_anno_min` `cf_sesso` (M/F) `cf_comune` (cod.catastale) `cf_valido` |

Il dispatch avviene per op in `PassaFiltro`: op CF → `valutaCF`, stringa → `valutaStringa`, booleano → `valutaBooleano`, data → `valutaData`, default → numerico (prova FloatMap → IntMap → StringMap).

Operatori senza valore (`vuoto`, `non_vuoto`, `vero`, `falso`, `cf_valido`) ignorano `FiltroConfig.Valore`.

**OR grouping**: `FiltroConfig.Gruppo int` — filtri con stesso numero > 0 sono OR'd tra loro; gruppo 0 = AND standalone. `applicaFiltri` in `engine.go` gestisce logica. `ApplicaFiltri` (esportata) usata da istruttoria.

**Gotcha `vuoto`/`non_vuoto` in FiltriFlag**: `vuoto` = flagga quando campo è vuoto (anomalia). Configurare `non_vuoto` per errore flaggerebbe quasi tutti. Campo assente da FloatMap/StringMap/IntMap → `PassaFiltro` ritorna `false` silenzioso, non panic.

### Filtri pre-estrazione istanza (`FiltriIstanzaConfig`)

`EngineConfig.Istanza FiltriIstanzaConfig` filtra le app per `Status` e `SubmittedAt` PRIMA dell'estrazione campi form. Configurabile in wizard step 4 sezione "Istanze". Applicato in `passaFiltriIstanza` in `engine.go`.

### Superset — chiavi speciali

`PostBuildSuperset` (wizard step 3) raccoglie anche:
- `"$app"."status"` → set di codici stato distinti
- `"$status_names"[code]` → nome API (machine-readable, es. `status_submitted`)
- `"$status_counts"[code]` → `["N"]` — conteggio istanze per stato

### Istruttoria — comportamento scan

`PostScansiona` (`POST /bandi/{id}/istruttoria/scansiona`):
1. **Reset `da_verificare`** (`db.ResetDaVerificare`) — ogni scan ricomincia; `approvata`/`esclusa` preservati; app con note/dati locali mantengono la riga ma con `motivi_json=[]`
2. Per ogni app che passa `filtri_istanza` (data+stato): estrae campi con nil extras → salva in `istruttorie_api_cache` (valori dichiarati puri)
3. Poi estrae con override locali applicati → valuta motivi e filtri ammissibilità
4. Auto-flag built-in (anche senza `Verifica.Attiva` se modalità fondi): corrispettivo=0 e CF richiedente mancante
5. Salva `UpsertIstruttoria` solo per app con motivi

**Gotcha**: calcolo bloccato da `da_verificare` rimasti con config filtri sbagliata → rescansiona per resettare.

### Modello dati istruttoria — due layer separati

| Tabella | Chiave | Contenuto |
|---------|--------|-----------|
| `istruttorie_dati` | `pratica_id` (cross-bando) | **Override operatore**: `{campo: valore}` — applicati al calcolo graduatoria via `EstraiRecordsConExtras` |
| `istruttorie_api_cache` | `(pratica_id, bando_id)` | **Valori dichiarati API**: aggiornati a ogni scan, mai mescolati con override |

`PostSaveDato` (`POST /bandi/{id}/istruttoria/{praticaID}/dato`):
- Salva override in `istruttorie_dati`
- Aggiorna campo in `istruttorie_api_cache` con valore API raw (ri-fetch OpenCity)
- `?ctx=dati`: risponde con `<span>` minimale invece del partial motivi — usato dalla pagina `/bandi/{id}/dati`

`GetIstruttoria` usa `GetAPICache` come fonte primaria per `campiDichiarati` (freschi da scan); snapshot ultima run come fallback.

### Package CF helper (`internal/graduatoria/cf/`)

Helper puri per codice fiscale italiano (formato: `AAABBB80A01H501U`):

```go
cf.EtaAnni(codice string) int        // età in anni interi a oggi (-1 se CF invalido)
cf.AnnoBirth(codice string) int      // anno di nascita (0 se invalido)
cf.Sesso(codice string) string       // "M" | "F" | ""
cf.ComuneNascita(codice string) string // codice catastale es. "H501"
cf.Valido(codice string) bool        // verifica checksum Agenzia Entrate
```

Tabella mese CF: `A`=1 `B`=2 `C`=3 `D`=4 `E`=5 `H`=6 `L`=7 `M`=8 `P`=9 `R`=10 `S`=11 `T`=12.
Giorno ≥ 41 → femmina (giorno reale = d−40). Anno YY: se YY ≤ annoCorrente%100 → 2000+YY, else 1900+YY.

CF di test (checksum corretto): `RSSMRA80A01H501U` (M, 01/01/1980, Roma H501), `BNCMRA99T61H501E` (F, 21/12/1999, Roma).

### Graduatoria serializzata in SQLite come JSON blob

`graduatorie_run.dati_json` contiene `json.Marshal(grad)` dove `grad` è `*graduatoria.Graduatoria`. I campi di `Istanza`, `RigaGraduatoria`, `GraduatoriaGruppo` non hanno tag JSON — vengono serializzati con il nome del campo Go. **Non rinominare quei campi** senza una migrazione dati.

`Graduatoria` ha una sola sezione:
- `Gruppi []*GraduatoriaGruppo` — una voce per tipologia (engine generic)
- `Escluse []RigaGraduatoria` — escluse per filtri o duplicati

Metodi helper su `Graduatoria`: `TotaleAmmesse`, `TotaleConRiserva`, `TotaleBudgetUsato`, `TotaleFuoriFondi`.

**Gotcha `TotaleFuoriFondi()`**: conta righe in `Gruppi` (non in `Escluse`) con `NoteEsclusione == "fondi esauriti" || "posti esauriti"`. Le escluse per filtri/deduplicazione stanno in `Escluse` — sono categorie distinte.

I template usano `{{if .Grad.Gruppi}}` per iterare i risultati.

### Wizard bando (`internal/web/handlers/bandi.go`)

Creare un bando richiede 7 step via wizard. Ogni step salva subito in DB — riprendibile da bozza. Alcuni step sono saltati automaticamente in base a `Modalita`.

| Step | Route | Cosa fa | Skip quando |
|------|-------|---------|-------------|
| 1 | `GET /bandi/nuovo` | Connette servizio OpenCity (HTMX) | — |
| 2 | `GET/POST /bandi/{id}/wizard/2` | **Tipo bando** (fondi/posti/ammissione/lista_attesa) | — |
| 3 | `GET/POST /bandi/{id}/wizard/3` | Mapping campi (flat JSON viewer → datalist) | — |
| 4 | `GET/POST /bandi/{id}/wizard/4` | Filtri (26+ operatori, dropdown per dominio) | — |
| 5 | `GET/POST /bandi/{id}/wizard/5` | Tipologie + ordinamento + deduplicazione | `ammissione`, `lista_attesa` → redirect a fine |
| 6 | `GET/POST /bandi/{id}/wizard/6` | Rimborso netto/lordo | tutto tranne `fondi` → redirect a fine |
| fine | `GET /bandi/{id}/wizard/fine` | Test engine (HTMX) + attiva → `stato_bando='attivo'` | — |

`saveEngineConfig` in `bandi.go` serializza la `EngineConfig` corrente e la salva in `bandi.engine_config`. I bandi attivi appaiono in `/bandi` per tutti gli operatori con accesso al servizio.

### Dashboard — split per ruolo (`internal/web/handlers/dashboard.go`)

`GetDashboard` sceglie il template in base al ruolo:
- Admin → `renderAdmin` → `dashboard_admin.html` (stats card + tabs `?stato=attivo|bozza|archiviato` + tabella CRUD bandi). Usa `db.CountBandiPerStato` per le stats.
- Operatore → `renderOperatore` → `dashboard_operatore.html` (cards bandi attivi + ultima run). Filtra per `op.CanAccessService`, passa `soloPublicate=true` a `db.ListRuns`.

Template `dashboard.html` eliminato — rinominato in `dashboard_operatore.html`.

### Workflow pubblicazione run

Le run vengono create in stato `'bozza'` (`graduatorie_run.stato`). Solo l'admin può pubblicarle (`POST /motori/{id}/run/{runID}/pubblica`). Gli operatori non-admin vedono solo le run `'pubblicata'` — `db.ListRuns` accetta un parametro opzionale `soloPublicate bool`.

`db.DeleteRunBozza` elimina una run solo se `stato='bozza'` (guard SQL: `DELETE ... WHERE stato='bozza'`, ritorna errore se 0 righe → run già pubblicata). Admin only via `PostEliminaRun` (`POST /bandi/{id}/run/{runID}/elimina`).

### Documento stampabile (`templates/run_stampa.html`)

`GET /motori/{id}/run/{runID}/stampa` — pagina con form checkbox colonne + tabella anonimizzata. Colonne configurabili via query param `?col=posizione&col=protocollo&col=cf&col=isee&col=importo&col=ammessa`.

FuncMap aggiuntive in `handlers/render.go`:
- `cfOscurato(cf)` → `"RSS***01Z"` (primi 3 + `***` + ultimi 3)
- `protocolloBreve(id)` → `"…" + ultimi 8 char`
- `hasCol(cols []string, col string) bool`
- `join(s []string, sep string) string`
- `statusLabel(code string) string` → label italiana per codice stato OpenCity (es. `"4000"` → `"In attesa istruttoria (4000)"`)
- `safeJSON(s string) template.JS` — stampa JSON grezzo senza escaping HTML
- `statoVerificaKey(campo string) string` → `"__stato_verifica_" + campo`
- `hasMotivoPrefix(motivi []string, prefix string) bool` — match prefisso in lista motivi
- `primoNonVuoto(vals ...string) string` — primo valore non vuoto

CSS `.filtro-row .form-control { flex: 1; min-width: 100px }` override per input a larghezza fissa: usare `style="width:52px;flex:0 0 52px;min-width:0"` direttamente sull'elemento.

CSS `@media print` in `static/style.css` nasconde `.no-print` e `.sidebar`; resetta `margin-left` su `.main-content`.

### Auth completamente delegata a OpenCity

Nessuna password locale. Flusso in `handlers/auth.go`:
1. `POST /lang/api/auth` → JWT (valido 10 giorni)
2. Decodifica payload JWT (base64 senza firma) → `id`, `roles: ["ROLE_OPERATORE"|"ROLE_ADMIN"]`
3. `GET /lang/api/users/{id}` → `enabled_services_ids` (UUID servizi abilitati per quell'operatore)
4. Admin = `user.Role == "admin"` **oppure** username in `ADMIN_USERNAMES` env
5. Sessione salvata in SQLite (`sessioni` table), cookie `session_id` HttpOnly SameSite=Strict
6. Middleware `Auth(db)` in `internal/web/middleware/auth.go` valida ogni request; inietta `*OperatorCtx` nel context

Rate limiting su `POST /login`: max 5 fallimenti per IP in 15 minuti → HTTP 429.

Un operatore vede solo i bandi il cui `service_id` è nel suo `enabled_services_ids`. Admin vede tutto.

### Template inheritance in Go html/template

Pattern usato in tutti i template:
```html
{{define "pagina.html"}}{{template "base" .}}{{end}}
{{define "title"}}Titolo{{end}}
{{define "content"}}<h1>...</h1>{{end}}
```
`base.html` usa `{{block "content" .}}`. `ParseGlob` carica tutti i `.html` in un unico `*template.Template` — i blocchi `define` in file separati si trovano automaticamente. **Non usare** `template.New(name).ParseFiles(...)` singolo: rompe il lookup cross-file.

`renderTemplate` in `handlers/render.go` casha i template con `sync.Once`. In sviluppo usare `GET /dev/reload-templates` (solo se `DEV=true`).

### Router Go 1.22+

`net/http` stdlib con pattern `METHOD /path/{param}`. Parametri via `r.PathValue("param")`. Nessun router esterno. `PUT` e `DELETE` non supportati nativamente dai form HTML — usare `POST /motori/{id}/archivia` al posto di `DELETE`.

Route principali (da `internal/web/server.go`):
```
GET  /bandi                               lista bandi
GET  /bandi/nuovo                         wizard step 1
POST /bandi/wizard/connetti               HTMX: lista servizi OpenCity
POST /bandi/wizard/crea                   crea bando bozza → redirect a /wizard/2
GET  /bandi/{id}/wizard/{step}            wizard step N (2-6, fine)
POST /bandi/{id}/wizard/{step}            salva step N
POST /bandi/{id}/wizard/test              HTMX: test engine
POST /bandi/{id}/wizard/attiva            attiva bando
GET  /bandi/{id}                          dettaglio bando
POST /bandi/{id}/duplica                  duplica
POST /bandi/{id}/archivia                 archivia
POST /bandi/{id}/run                      calcola graduatoria
GET  /bandi/{id}/run/{runID}              dettaglio run
GET  /bandi/{id}/run/{runID}/{anno}/{tipo}   tabella (solo tipo="escluse" attivo)
GET  /bandi/{id}/run/{runID}/gruppo/{nome}   tabella per gruppo
GET  /bandi/{id}/run/{runID}/export/{anno}/{tipo}  CSV (solo tipo="escluse" attivo)
GET  /bandi/{id}/run/{runID}/export/gruppo/{nome}  CSV gruppo
GET  /bandi/{id}/run/{runID}/export/mapping/{mapID}  CSV da export mapping configurato
POST /bandi/{id}/run/{runID}/pubblica     pubblica run (admin only)
POST /bandi/{id}/run/{runID}/elimina      elimina run bozza (admin only)
GET  /bandi/{id}/run/{runID}/stampa       documento stampabile
POST /bandi/{id}/run/{runID}/approva-batch
POST /bandi/{id}/run/{runID}/rifiuta-batch
GET  /bandi/{id}/istruttoria              istruttoria pre-calcolo (flagged apps)
GET  /bandi/{id}/dati                     dati locali tutte le domande scansionate
POST /bandi/{id}/istruttoria/scansiona    scansiona istanze → popola istruttorie + api_cache
POST /bandi/{id}/istruttoria/batch        azione batch istruttoria
POST /bandi/{id}/istruttoria/{praticaID}/dato  salva override locale + ri-valuta motivi
POST /bandi/{id}/istruttoria/{praticaID}/nota  salva nota lavoro (HTMX)
POST /bandi/{id}/istruttoria/{praticaID}/riapri  → da_verificare
GET  /bandi/{id}/istruttoria/{praticaID}/collega-form  HTMX: select pratiche stesso CF altri bandi
POST /bandi/{id}/istruttoria/{praticaID}/collega       salva link manuale cross-bando
POST /bandi/{id}/istruttoria/{praticaID}/scollega/{collegamentoID}  rimuove link
GET  /bandi/{id}/export-mappings          lista template CSV (admin)
POST /bandi/{id}/export-mappings          crea template
GET  /bandi/{id}/export-mappings/{mapID}  edit template
POST /bandi/{id}/export-mappings/{mapID}  salva template
POST /bandi/{id}/export-mappings/{mapID}/delete
GET  /audit                               audit trail
GET  /dev/reload-templates                solo se DEV=true
```

`bandoIDFromPath(r)` e `parseFloat(s)` in `handlers/helpers.go`.

`db.ListRuns(db, bandoID, soloPublicate ...bool)` — terzo parametro variadic: se `true`, filtra solo run `'pubblicata'` (usato in `renderOperatore`; admin passa `false`).

## OpenCity API — Istanza Montesilvano

**Base URL:** `https://service.comune.montesilvano.pe.it`  
**Auth:** `POST /lang/api/auth` body `{"username":"...","password":"..."}` → `{"token":"<jwt>"}`  
**JWT**: validità 10 giorni, header `Authorization: Bearer <token>`  
**`version=2` OBBLIGATORIO** in tutte le query — v1 appiattisce i campi Form.IO in stringhe.

**ATTENZIONE**: il param `order` accetta solo `creationTime`. Qualsiasi altro valore → HTTP 400.

### Endpoint rilevanti

| Metodo | Path | Note |
|--------|------|------|
| `GET` | `/lang/api/applications` | paginato; params: `service_id`, `status`, `offset`, `limit` |
| `POST` | `/lang/api/applications/{id}/transition/accept` | body `{"message":"..."}` |
| `POST` | `/lang/api/applications/{id}/transition/reject` | body `{"message":"..."}` |
| `GET` | `/lang/api/users/{id}` | `version=2`; ritorna `enabled_services_ids` |

### Status code istanza

| Codice | Significato |
|--------|-------------|
| `2000` | bozza |
| `3000` | inviata |
| `4000` | pending (in attesa istruttoria) |
| `9000` | approvata |
| `20000` | ritirata (esclusa automaticamente dai filtri) |

`Application.StatusName` dall'API è machine-readable (es. `status_submitted`) — non localizzato. Usare `statusLabel` funcmap per label italiana in template.

## Struttura `data` — Rimborso rette e mense (service_id: `5756cd98-7fe6-4818-bad8-69a2c843b546`)

Campi non ovvi (ancora rilevanti per configurare il mapping JSON nel wizard):

```
data.ordinary_economic_situation_indicator.isee           float64  — valore ISEE
data.ordinary_economic_situation_indicator.meta.signature string   — non vuota = certificato PDND
data.ordinary_economic_situation_indicator.meta.source    string   — "INPS"
data.ordinary_economic_situation_indicator.valid_until    string   — "31/12/2026"

data.anni[].tiporichiesta    "rette" | "mensa"
data.anni[].annualita1       20232024 | 20242025
data.anni[].corrispettivo    float64  — importo lordo
data.anni[].importoDelBeneficioRicevuto  float64  — Bonus Nidi già percepito

data.iban.iban        string  ⚠️ chiave JSON è "iban" non "iban2"
data.select_child     string  — CF figlio
data.children.children[].tax_id  — figli nucleo ANPR
```

## Algoritmo Graduatoria — Art. 6 Avviso FSE+ (Det. n.122 del 16.03.2026)

Ora configurato tramite `EngineConfig` (modalita=fondi). Parametri storici:

**Budget**: €71.096,37 totale → €35.548,18 per annualità  
**ISEE max**: €40.000  
**Scadenza domande**: 24 aprile 2026 ore 23:59 Europe/Rome

**Risultati giugno 2026** (benchmark):

| Annualità | Rette ammesse | Mense ammesse | Fuori fondi | Budget usato |
|-----------|--------------|--------------|-------------|-------------|
| 20232024 | 2 | 101 | 53 | €35.548,18 |
| 20242025 | 6 | 101 | 130 | €35.548,18 |
| Escluse | 49 | — | — | — |

## Servizi disponibili

| Nome | ID | Istanze |
|------|----|---------|
| Rette e mense | `5756cd98-7fe6-4818-bad8-69a2c843b546` | 332 |
| Libri di testo | `aeffaacf-adad-461b-83f0-ee3d95d87f31` | 629 |
| Centri estivi | `05a37702-0710-43eb-8165-3a11fc766f49` | 161 |
| Viaggio riabilitazione | `10987e1d-afa3-4b53-83fb-ef2c2db04cdb` | 7 |

Tutti i servizi usano `engine_type = "generic"` — configurabili via wizard senza codice.

## Debug DB in produzione

Container distroless — nessuna shell né `sqlite3`. Per accedere al DB:
```bash
docker cp opencity-backend-gestionale-1:/data/gestionale.db /tmp/gestionale.db
python3 -c "import sqlite3; db=sqlite3.connect('/tmp/gestionale.db'); [print(r) for r in db.execute('SELECT ...')]"
```

### Export mapping (`internal/db/export_mappings.go`, `handlers/export_mappings.go`)

Template CSV configurabili per-bando. `ExportColonna` ha tre sorgenti:
- `"sistema"` — campo dal run snapshot (`posizione`, `protocollo`, `cognome`, `nome`, `cf_richiedente`, `tipologia`, `importo`, `annualita`, `stato_app`)
- `"mappato"` — campo da `Istanza.CampiMappati` (estratto dal calcolo)
- `"raw"` — path dot-notation su `app.Data` (ri-fetch OpenCity al momento dell'export)

`GetExportCSVMapped` streama CSV filtrato per `FiltroStati` (vuoto = tutti). Sorgente `"raw"` richiede `FetchAllApplications`.

L'editor colonne usa il superset browser (stesso JS del wizard step 3).

### Pagina dati locali (`/bandi/{id}/dati`)

Mostra tutte le domande scansionate (`istruttorie_api_cache`). Badge per pratica:
- `ammessa` / `fuori_fondi` — da snapshot ultima run
- `da_verificare` / `approvata` / `esclusa` — da `istruttorie`
- `non_rientrante` — passa `filtri_istanza` ma non i filtri di merito (ISEE ecc.); potrebbe rientrare dopo override ISEE

Form dinamico: ogni campo da `EngineConfig.Mapping` ha input con `type="text" inputmode="decimal|numeric"` per float/int (mai `type="number"` — locale italiana causa `element.value=""` su valori con virgola). Il tasto "Salva" chiama `salvaTutto()` via `fetch()` nativo — non HTMX (`hx-vals` non serializza `campo` correttamente in questa versione HTMX). `PostSaveDato` risponde con `<span>` minimale quando `?ctx=dati`. Valore vuoto → `delete(dati, campo)` in `SaveDatoIstruttoria`.

**Collegamento manuale cross-bando**: Badge "Anche in" verde per link espliciti tra pratiche di soggetti diversi in bandi diversi (vs badge automatico = stesso `pratica_id`). `GetPraticheCollegabili` mostra "+ Collega" solo se esiste altra pratica con stesso `richiedente_cf` in bando diverso E `pratica_id` diverso (esclude caso badge automatico). `TrovaPraticheStessoCF` legge CF da `istruttorie_api_cache.dati_json` (prova chiave `richiedente_cf` poi `richiedente`). `AddCollegamento` normalizza: bando_id minore va in posizione A — evita duplicati con UNIQUE constraint. `GetCollegamenti` usa UNION ALL bidirezionale per leggere da entrambi i lati della riga. `GetCollegaForm` → fragment HTMX inline (`fmt.Fprintf`), non usa template file.

## Schema SQLite (`internal/db/schema.sql` + migrazioni in `db.Open()`)

| Tabella | Descrizione |
|---------|-------------|
| `bandi` | Configurazione bando: service_id, budget, ISEE max, engine_config, stato_bando |
| `graduatorie_run` | Snapshot `Graduatoria` JSON in `dati_json`; `stato` (`bozza`\|`pubblicata`) |
| `istruttorie` | Flag per-bando per-pratica: stato (`da_verificare`\|`approvata`\|`esclusa`), motivi, app_status, nota_lavoro |
| `istruttorie_dati` | Override operatore cross-bando: `pratica_id PK`, JSON `{campo: valore}` — solo campi sovrascritti dall'operatore |
| `istruttorie_api_cache` | Valori dichiarati API: `(pratica_id, bando_id) PK`, JSON campi estratti durante scan — mai mescolati con override |
| `note_lavoro` | Note operatore per-bando per-pratica: `(bando_id, pratica_id) PK`, `nota` — tabella separata da `istruttorie` per evitare row `da_verificare` fantasma |
| `pratiche_collegate` | Link manuale cross-bando: `(bando_id_a, pratica_id_a, bando_id_b, pratica_id_b)` normalizzato (bando_id minore in posizione A) con UNIQUE constraint |
| `export_mappings` | Template CSV per-bando: colonne_json, filtro_stati |
| `audit_actions` | Ogni approve/reject/calcola/pubblica con esito e messaggio |
| `sessioni` | JWT OpenCity + metadati operatore; scade dopo 10 giorni |

Tutte le nuove tabelle/colonne aggiunte via `ALTER TABLE` idempotente in `db.Open()` — nessuna migrazione manuale necessaria.
