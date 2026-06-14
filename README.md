# opencity-gestionale

Gestionale web per la gestione di bandi FSE+ e graduatorie su [OpenCity Italia](https://opencityitalia.it/) (La Stanza del Cittadino).

Sviluppato per il **Comune di Montesilvano** — adattabile a qualsiasi ente che utilizzi OpenCity Italia.

## Funzionalità

- **Wizard motore di calcolo** — configura un nuovo bando in 6 step guidati: connetti servizio OpenCity → mappa campi → filtri → tipologie/ordinamento/deduplicazione → rimborso → test e attiva
- **Engine generico configurabile** — qualsiasi bando FSE+ configurabile via JSON (mapping campi, filtri, de-duplicazione per figlio/anno/tipo, budget per tipologia)
- **Calcolo graduatorie** — fetch istanze da OpenCity API v2, calcolo con algoritmo configurabile
- **Documento stampabile** — genera prospetto graduatoria anonimizzato (CF oscurato) con colonne configurabili, stampa direttamente da browser → PDF
- **Bulk approve/reject** — approva o rifiuta pratiche in massa con messaggio personalizzato direttamente su OpenCity
- **Audit trail** — ogni azione (calcolo, approvazione, rifiuto, pubblicazione) tracciata con operatore, esito e messaggio
- **Auth delegata a OpenCity** — le credenziali sono quelle del portale operatori, nessuna gestione password separata
- **Export CSV** — graduatorie esportabili con separatore `;` compatibile con Excel italiano
- **CLI batch** — modalità non interattiva per produzione CSV + prospetto HTML operatori
- **Workflow pubblicazione** — le graduatorie nascono in bozza (visibili solo agli admin), vengono pubblicate quando pronte

## Stack

| Layer | Tecnologia |
|-------|-----------|
| Backend | Go 1.22+ `net/http` stdlib |
| Frontend | HTMX + `html/template` |
| Database | SQLite (`modernc.org/sqlite` — pure Go, no CGO) |
| Deploy | Docker/Podman rootless |
| CI/CD | GitHub Actions → GHCR |

## Avvio rapido

### Prerequisiti

- Go 1.22+
- Docker / Podman

### Sviluppo locale

```bash
git clone https://github.com/Comune-di-Montesilvano/opencity-gestionale.git
cd opencity-gestionale

cp .env.example .env
# modifica .env con le credenziali reali
go run ./cmd/server
```

Apri `http://localhost:8080` e accedi con le credenziali OpenCity.

### Docker

```bash
cp .env.example .env
# modifica .env con SECRET_KEY e credenziali reali
docker compose up -d
```

### CLI batch (CSV + HTML)

```bash
# richiede OPENCITY_USERNAME e OPENCITY_PASSWORD in .env
go run ./cmd/batch
# output in ./output/
```

## Configurazione

| Variabile | Default | Descrizione |
|-----------|---------|-------------|
| `OPENCITY_BASE_URL` | — | **Obbligatoria.** URL base istanza OpenCity |
| `SECRET_KEY` | — | **Obbligatoria.** Min 32 caratteri. Genera con `openssl rand -hex 32` |
| `ADMIN_USERNAMES` | — | Username admin separati da virgola |
| `DB_PATH` | `gestionale.db` | Path file SQLite |
| `PORT` | `8080` | Porta HTTP |
| `TRUST_PROXY` | `false` | `true` se dietro reverse proxy HTTPS (abilita cookie `Secure`) |
| `OPENCITY_USERNAME` | — | Solo per `cmd/batch`: username operatore API |
| `OPENCITY_PASSWORD` | — | Solo per `cmd/batch`: password operatore API |

## Prima configurazione

Al primo avvio il database è vuoto. Accedi con un account admin e vai su `/motori/nuovo`:

1. **Connetti servizio** — verifica connettività verso OpenCity, carica lista servizi disponibili
2. **Configura** — nome motore, budget totale, ISEE massimo
3. **Mappa campi** — visualizza il JSON reale delle istanze (campo → valore) e mappa i campi logici
4. **Filtri** — aggiungi criteri di esclusione automatica (es. ISEE > 40000)
5. **Tipologie** — definisci le categorie di rimborso con priorità e budget
6. **Rimborso** — scegli modalità netto/lordo e campi sorgente
7. **Test + Attiva** — esegui il motore su una istanza campione, poi attiva

Il motore è disponibile agli operatori non appena attivato.

## Deploy in produzione

Il gestionale è pensato per girare dietro un reverse proxy esistente (nginx, Caddy) che gestisce TLS.

```bash
# Sul server
cp .env.example .env
chmod 600 .env
# Modifica .env

docker compose up -d
```

Per aggiornare: crea un tag Git → CI pubblica nuova immagine su GHCR → pull sul server.

```bash
git tag v1.0.0 && git push origin v1.0.0
# attendi CI (~2 min)
ssh server "cd /opt/gestionale && docker compose pull && docker compose up -d"
```

## Aggiungere un nuovo engine di calcolo

L'engine `generic` copre la maggior parte dei bandi FSE+. Se serve logica custom:

1. Crea `internal/graduatoria/<nome>/engine.go`
2. Implementa `graduatoria.ServiceEngine` (`Name`, `Calcola`, `CSVHeaders`, `CSVRecord`)
3. Registra via `func init() { graduatoria.Register(&Engine{}) }`
4. Aggiungi blank import in `cmd/server/main.go` e `cmd/batch/main.go`

Engine disponibili:

| Engine | Uso |
|--------|-----|
| `generic` | Qualsiasi bando FSE+ — configurabile via wizard senza codice |
| `mense_rette` | Engine legacy hardcoded per rette e mense FSE+ Abruzzo 2026 |

## Licenza

[EUPL 1.2](LICENSE) — Licenza Pubblica dell'Unione europea
