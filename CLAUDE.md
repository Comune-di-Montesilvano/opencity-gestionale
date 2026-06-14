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

I template usano `{{if .Grad.Gruppi}}` per iterare i risultati.

### Wizard motore (`internal/web/handlers/motori.go`)

Creare un motore richiede 7 step via wizard. Ogni step salva subito in DB — riprendibile da bozza. Alcuni step sono saltati automaticamente in base a `Modalita`.

| Step | Route | Cosa fa | Skip quando |
|------|-------|---------|-------------|
| 1 | `GET /motori/nuovo` | Connette servizio OpenCity (HTMX) | — |
| 2 | `GET/POST /motori/{id}/wizard/2` | **Tipo bando** (fondi/posti/ammissione/lista_attesa) | — |
| 3 | `GET/POST /motori/{id}/wizard/3` | Mapping campi (flat JSON viewer → datalist) | — |
| 4 | `GET/POST /motori/{id}/wizard/4` | Filtri (26+ operatori, dropdown per dominio) | — |
| 5 | `GET/POST /motori/{id}/wizard/5` | Tipologie + ordinamento + deduplicazione | `ammissione`, `lista_attesa` → redirect a fine |
| 6 | `GET/POST /motori/{id}/wizard/6` | Rimborso netto/lordo | tutto tranne `fondi` → redirect a fine |
| fine | `GET /motori/{id}/wizard/fine` | Test engine (HTMX) + attiva → `stato_motore='attivo'` | — |

`saveEngineConfig` in `motori.go` serializza la `EngineConfig` corrente e la salva in `bandi.engine_config`. I motori attivi appaiono in `/motori` per tutti gli operatori con accesso al servizio.

### Workflow pubblicazione run

Le run vengono create in stato `'bozza'` (`graduatorie_run.stato`). Solo l'admin può pubblicarle (`POST /motori/{id}/run/{runID}/pubblica`). Gli operatori non-admin vedono solo le run `'pubblicata'` — `db.ListRuns` accetta un parametro opzionale `soloPublicate bool`.

### Documento stampabile (`templates/run_stampa.html`)

`GET /motori/{id}/run/{runID}/stampa` — pagina con form checkbox colonne + tabella anonimizzata. Colonne configurabili via query param `?col=posizione&col=protocollo&col=cf&col=isee&col=importo&col=ammessa`.

FuncMap aggiuntive in `handlers/render.go`:
- `cfOscurato(cf)` → `"RSS***01Z"` (primi 3 + `***` + ultimi 3)
- `protocolloBreve(id)` → `"…" + ultimi 8 char`
- `hasCol(cols []string, col string) bool`
- `join(s []string, sep string) string`

CSS `@media print` in `static/style.css` nasconde `.no-print` e `.navbar`.

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
GET  /motori                              lista motori
GET  /motori/nuovo                        wizard step 1
POST /motori/wizard/connetti              HTMX: lista servizi OpenCity
POST /motori/wizard/crea                  crea motore bozza → redirect a /wizard/2
GET  /motori/{id}/wizard/{step}           wizard step N (2-6, fine)
POST /motori/{id}/wizard/{step}           salva step N
POST /motori/{id}/wizard/test             HTMX: test engine
POST /motori/{id}/wizard/attiva           attiva motore
GET  /motori/{id}                         dettaglio motore
POST /motori/{id}/duplica                 duplica
POST /motori/{id}/archivia                archivia
POST /motori/{id}/run                     calcola graduatoria
GET  /motori/{id}/run/{runID}             dettaglio run
GET  /motori/{id}/run/{runID}/{anno}/{tipo}  tabella (solo tipo="escluse" attivo)
GET  /motori/{id}/run/{runID}/gruppo/{nome}  tabella per gruppo
GET  /motori/{id}/run/{runID}/export/{anno}/{tipo}  CSV (solo tipo="escluse" attivo)
GET  /motori/{id}/run/{runID}/export/gruppo/{nome}  CSV gruppo
POST /motori/{id}/run/{runID}/pubblica    pubblica run (admin only)
GET  /motori/{id}/run/{runID}/stampa      documento stampabile
POST /motori/{id}/run/{runID}/approva-batch
POST /motori/{id}/run/{runID}/rifiuta-batch
GET  /audit                               audit trail
GET  /dev/reload-templates                solo se DEV=true
```

`bandoIDFromPath(r)` e `parseFloat(s)` in `handlers/helpers.go` — usati da graduatoria e motori handler.

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
| `4000` | pending |
| `20000` | ritirata (esclusa automaticamente dai filtri) |

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

## Schema SQLite (`internal/db/schema.sql`)

- `bandi`: configurazione bando per servizio (budget, ISEE max, engine_type, engine_config, scadenza)
- `graduatorie_run`: snapshot `Graduatoria` come JSON blob in `dati_json`; colonna `stato` (`'bozza'`|`'pubblicata'`)
- `audit_actions`: ogni approve/reject/calcola/pubblica con esito e messaggio
- `sessioni`: JWT OpenCity + metadati operatore; scade dopo 10 giorni

La colonna `stato` in `graduatorie_run` è aggiunta via `ALTER TABLE` idempotente in `db.Open()` per compatibilità con DB pre-esistenti.
