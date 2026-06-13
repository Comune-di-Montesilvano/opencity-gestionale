# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Progetto

Gestionale web per il Comune di Montesilvano: fetch istanze da OpenCity Italia (La Stanza del Cittadino), calcolo graduatorie FSE+, bulk approve/reject con audit trail. Il CSV nativo di OpenCity tronca campi Form.IO annidati — da qui la necessità di chiamare l'API con `version=2`.

Due binari distinti: `cmd/server` (web gestionale) e `cmd/batch` (CLI one-shot → CSV + HTML).

## Comandi

```bash
go build ./...
go test ./...
go vet ./...

# CLI batch: fetch → graduatoria → CSV + HTML in output/
go run ./cmd/batch

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

## Architettura non ovvia

### Multi-engine per servizi diversi

`ServiceEngine` interface in `internal/graduatoria/service.go`. Ogni engine si auto-registra via `func init()`:

```go
func init() { graduatoria.Register(&Engine{}) }
```

Il binario deve importare l'engine con blank import per triggerare `init()`. Sia `cmd/server/main.go` che `cmd/batch/main.go` importano entrambi:
```go
import _ "opencity-gestionale/internal/graduatoria/generic"
import _ "opencity-gestionale/internal/graduatoria/mense"
```

**Engine registrati**:
- `mense_rette` — engine legacy hardcoded per bando rette/mense FSE+ 2026
- `generic` — engine configurabile via `engine_config` JSON (qualsiasi bando)

**Aggiungere un nuovo engine**: crea `internal/graduatoria/<nome>/engine.go`, implementa `ServiceEngine`, aggiungi blank import in entrambi i binari.

### Engine generico (`internal/graduatoria/generic/`)

Supporta qualsiasi bando FSE+ tramite configurazione JSON in `bandi.engine_config`. La struttura `EngineConfig` è definita in `internal/graduatoria/config.go`.

**Struttura `EngineConfig`**:
```json
{
  "mapping": {
    "isee":   { "path": "ordinary_economic_situation_indicator.isee", "tipo": "float" },
    "tipo":   { "path": "tiporichiesta", "expand": true },
    "data_presentazione": { "path": "$app:submitted_at", "tipo": "time" }
  },
  "espansione": "anni",
  "filtri": [
    { "campo": "isee", "op": "<=", "valore": 40000 },
    { "campo": "corrispettivo_netto", "op": ">", "valore": 0 }
  ],
  "deduplicazione": { "attiva": true, "chiave": ["figlio_cf", "annualita", "tipo"] },
  "ordinamento": [{ "campo": "isee", "dir": "asc" }],
  "tipologie": [
    { "nome": "rette", "campo": "tipo", "valore": "rette", "priorita": 1, "budget": { "tipo": "residuo" } }
  ],
  "rimborso": { "tipo": "netto", "campo_lordo": "corrispettivo", "campo_deduzione": "beneficio" }
}
```

**Sintassi path**:
- `"foo.bar.baz"` — navigazione oggetto annidato nel payload `data`
- `"$app:submitted_at"` — campo top-level di `Application` (id, submitted_at, protocol_number, status)
- `"tipo": "count"` — lunghezza array
- `"expand": true` — campo relativo a ogni elemento dell'array `espansione`

**Campi derivati**: `corrispettivo_netto = corrispettivo - beneficio` calcolato automaticamente se `rimborso.tipo == "netto"`.

**Budget tipologia**: `"tipo": "residuo"` (tutto il residuo dopo priorità superiori) | `"percentuale"` | `"fisso"`.

Il package `internal/graduatoria/extractor/` espone `Float`, `Str`, `Count`, `Time`, `ArrayElements`, `AppField` per navigazione dot-notation su `json.RawMessage`.

Il tipo `Record` (`internal/graduatoria/record.go`) raccoglie i campi estratti (FloatMap, StringMap, IntMap, TimeMap) e si converte in `*Istanza` via `ToIstanza()` per compatibilità con i template.

### Graduatoria serializzata in SQLite come JSON blob

`graduatorie_run.dati_json` contiene `json.Marshal(grad)` dove `grad` è `*graduatoria.Graduatoria`. I campi di `Istanza`, `RigaGraduatoria`, `GraduatoriaAnnualita`, `GraduatoriaGruppo` non hanno tag JSON — vengono serializzati con il nome del campo Go. **Non rinominare quei campi** senza una migrazione dati, altrimenti le run salvate non si deserializzano.

`Graduatoria` ha due sezioni distinte:
- `PerAnno []*GraduatoriaAnnualita` — usata dall'engine `mense_rette` (split per annualità + tipo)
- `Gruppi []*GraduatoriaGruppo` — usata dall'engine `generic` (una voce per tipologia)

I template controllano quale campo è popolato: `{{if .Grad.PerAnno}}` / `{{if .Grad.Gruppi}}`.

### Workflow pubblicazione run

Le run vengono create in stato `'bozza'` (`graduatorie_run.stato`). Solo l'admin può pubblicarle (`POST /bandi/{id}/run/{runID}/pubblica`). Gli operatori non-admin vedono solo le run `'pubblicata'` — `db.ListRuns` accetta un parametro opzionale `soloPublicate bool`.

### Auth completamente delegata a OpenCity

Nessuna password locale. Flusso in `handlers/auth.go`:
1. `POST /lang/api/auth` → JWT (valido 10 giorni)
2. Decodifica payload JWT (base64 senza firma) → `id`, `roles: ["ROLE_OPERATORE"|"ROLE_ADMIN"]`
3. `GET /lang/api/users/{id}` → `enabled_services_ids` (UUID servizi abilitati per quell'operatore)
4. Admin = `user.Role == "admin"` **oppure** username in `ADMIN_USERNAMES` env
5. Sessione salvata in SQLite (`sessioni` table), cookie `session_id` HttpOnly SameSite=Strict
6. Middleware `Auth(db)` in `internal/web/middleware/auth.go` valida ogni request; inietta `*OperatorCtx` nel context

Un operatore vede solo i bandi il cui `service_id` è nel suo `enabled_services_ids`. Admin vede tutto.

### Template inheritance in Go html/template

Pattern usato in tutti i template:
```html
{{define "pagina.html"}}{{template "base" .}}{{end}}
{{define "title"}}Titolo{{end}}
{{define "content"}}<h1>...</h1>{{end}}
```
`base.html` usa `{{block "content" .}}`. `ParseGlob` carica tutti i `.html` in un unico `*template.Template` — i blocchi `define` in file separati si trovano automaticamente. **Non usare** `template.New(name).ParseFiles(...)` singolo: rompe il lookup cross-file.

`renderTemplate` in `handlers/render.go` casha i template con `sync.Once`. In sviluppo, il reload richiede riavvio del server (o chiamare `ReloadTemplates()` non ancora esposto via route).

### ParseIstanze: un'istanza → N righe

`data.anni[]` può avere 1 o 2 voci (111/332 istanze hanno 2 annualità). `ParseIstanze()` in `engine.go` espande ogni voce in una `*Istanza` separata con campi `Annualita` e `TipoRichiesta` popolati. La de-duplicazione avviene su chiave `(FiglioSelezionatoCF, Annualita, TipoRichiesta)` — un figlio può avere un solo rimborso per tipo/anno indipendentemente da quale genitore ha presentato.

### Router Go 1.22+

`net/http` stdlib con pattern `METHOD /path/{param}`. Parametri via `r.PathValue("param")`. Nessun router esterno. `PUT` e `DELETE` non supportati nativamente dai form HTML — usare `POST /bandi/{id}/disattiva` al posto di `DELETE`.

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
| `20000` | ritirata (esclusa automaticamente dall'algoritmo) |

## Struttura `data` — Rimborso rette e mense (service_id: `5756cd98-7fe6-4818-bad8-69a2c843b546`)

Campi non ovvi mappati in `internal/opencity/models.go` → `MenseData`:

```
data.ordinary_economic_situation_indicator.isee           float64  — valore ISEE
data.ordinary_economic_situation_indicator.meta.signature string   — non vuota = certificato PDND
data.ordinary_economic_situation_indicator.meta.source    string   — "INPS"
data.ordinary_economic_situation_indicator.valid_until    string   — "31/12/2026"
data.ordinary_economic_situation_indicator.dsu_protocol_number string

data.anni[].tiporichiesta    "rette" | "mensa"
data.anni[].annualita1       20232024 | 20242025
data.anni[].corrispettivo    float64  — importo lordo
data.anni[].importoDelBeneficioRicevuto  float64  — Bonus Nidi già percepito

data.iban.iban        string  ⚠️ chiave JSON è "iban" non "iban2"
data.iban.iban_check  string  — es. "Valido"

data.select_child     string  — CF figlio (può essere vuoto)
data.children.children[].tax_id  — figli nucleo ANPR
```

## Algoritmo Graduatoria — Art. 6 Avviso FSE+ (Det. n.122 del 16.03.2026)

**Budget**: €71.096,37 totale → €35.548,18 per annualità (20232024 e 20242025)  
**ISEE max**: €40.000  
**Scadenza domande**: 24 aprile 2026 ore 23:59 Europe/Rome

**Ordine priorità per annualità:**
1. Rette (ISEE asc → num figli desc) fino a esaurimento budget annualità
2. Mense con budget residuo dopo rette

**Corrispettivo netto** = `corrispettivo - importoDelBeneficioRicevuto` (Bonus Nidi INPS detratto)  
Ultimo beneficiario utile → rimborso parziale se fondi insufficienti.

**Bonus Nidi coerenza**: soglia €3.000 annui (11 mesi × €272,73). Anomalia se dichiarato > €3.000 — colonna `bonus_nidi_coerente = "no"` nel CSV.

**Risultati giugno 2026** (benchmark per regression test):

| Annualità | Rette ammesse | Mense ammesse | Fuori fondi | Budget usato |
|-----------|--------------|--------------|-------------|-------------|
| 20232024 | 2 | 101 | 53 | €35.548,18 |
| 20242025 | 6 | 101 | 130 | €35.548,18 |
| Escluse | 49 | — | — | — |

Escluse: 22 ritirate, 17 duplicati, 5 ISEE=0, 5 corrispettivo netto=0.

## Servizi disponibili

| Nome | ID | Istanze |
|------|----|---------|
| **Rette e mense** (engine: `mense_rette`) | `5756cd98-7fe6-4818-bad8-69a2c843b546` | 332 |
| Libri di testo | `aeffaacf-adad-461b-83f0-ee3d95d87f31` | 629 |
| Centri estivi | `05a37702-0710-43eb-8165-3a11fc766f49` | 161 |
| Viaggio riabilitazione | `10987e1d-afa3-4b53-83fb-ef2c2db04cdb` | 7 |

## Schema SQLite (`internal/db/schema.sql`)

- `bandi`: configurazione bando per servizio (budget, ISEE max, engine_type, engine_config, scadenza)
- `graduatorie_run`: snapshot `Graduatoria` come JSON blob in `dati_json`; colonna `stato` (`'bozza'`|`'pubblicata'`)
- `audit_actions`: ogni approve/reject/calcola/pubblica con esito e messaggio
- `sessioni`: JWT OpenCity + metadati operatore; scade dopo 10 giorni

La colonna `stato` in `graduatorie_run` è aggiunta via `ALTER TABLE` idempotente in `db.Open()` per compatibilità con DB pre-esistenti.

## Output CLI batch (`output/`)

Separatore CSV `;`, encoding UTF-8.

**Badge verifica** nel prospetto HTML (logica in `cmd/batch/output_html.go` → `flagsRiga`):

| Badge | Condizione |
|-------|------------|
| ISEE non PDND | `!ISEEVerificato \|\| ISEEFonte==""` |
| tutore legale | `for_whom` contiene "tutore" |
| Bonus Nidi ⚠ | `bonusNidiCoerente == "no"` |
| IBAN: … | `IBANCheck != "" && != "Valido"` |
| CF figlio mancante | `FiglioSelezionatoCF == ""` |
| TARDIVA | `SubmittedAt > 2026-04-24T23:59:59+02:00` |

Link operatori: `https://service.comune.montesilvano.pe.it/lang/it/operatori/{id}/detail`
