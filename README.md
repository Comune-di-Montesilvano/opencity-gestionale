# opencity-gestionale

Gestionale web per la gestione di bandi FSE+ e graduatorie su [OpenCity Italia](https://opencityitalia.it/) (La Stanza del Cittadino).

Sviluppato per il **Comune di Montesilvano** — adattabile a qualsiasi ente che utilizzi OpenCity Italia.

## Funzionalità

- **Calcolo graduatorie** — fetch istanze da OpenCity API v2, calcolo con algoritmo configurabile (budget, ISEE, de-duplicazione per figlio/anno/tipo)
- **Multi-bando** — ogni servizio OpenCity ha il suo bando con parametri indipendenti
- **Bulk approve/reject** — approva o rifiuta pratiche in massa con messaggio personalizzato direttamente su OpenCity
- **Audit trail** — ogni azione (calcolo, approvazione, rifiuto) tracciata con operatore, esito e messaggio
- **Auth delegata a OpenCity** — le credenziali sono quelle del portale operatori, nessuna gestione password separata
- **Export CSV** — graduatorie esportabili con separatore `;` compatibile con Excel italiano
- **CLI batch** — modalità non interattiva per produzione CSV + prospetto HTML operatori

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

DB_PATH=./dev.db \
OPENCITY_BASE_URL=https://service.comune.montesilvano.pe.it \
SECRET_KEY=devdevdevdevdevdevdevdevdevdevdev \
ADMIN_USERNAMES=tuousername \
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

## Deploy in produzione

Il gestionale è pensato per girare dietro un reverse proxy esistente (nginx, Caddy) che gestisce TLS.

```bash
# Sul server
cp .env.example .env
chmod 600 .env
# Modifica .env

./deploy.sh  # docker compose pull && up -d
```

Per aggiornare: crea un tag Git → CI pubblica nuova immagine su GHCR → `./deploy.sh` sul server.

```bash
git tag v1.0.0 && git push origin v1.0.0
# attendi CI (~2 min)
ssh server "cd /opt/gestionale && ./deploy.sh"
```

## Prima configurazione (setup wizard)

Al primo avvio il database è vuoto. Il wizard è accessibile da `/setup` dopo il login con un account admin:

1. **Verifica connessione** — testa la connettività verso OpenCity e carica la lista servizi disponibili
2. **Configura bandi** — per ogni servizio: nome, budget totale, ISEE massimo, scadenza, engine di calcolo
3. **Salva** → redirect alla dashboard

## Aggiungere un nuovo engine di calcolo

Ogni tipologia di bando ha il suo engine. Per aggiungerne uno:

1. Crea `internal/graduatoria/<nome>/engine.go`
2. Implementa `graduatoria.ServiceEngine` (`Name`, `Calcola`, `CSVHeaders`, `CSVRecord`)
3. Registra via `func init() { graduatoria.Register(&Engine{}) }`
4. Aggiungi blank import in `cmd/server/main.go` e `cmd/batch/main.go`

Engine disponibili:

| Engine | Servizio |
|--------|---------|
| `mense_rette` | Rimborso spese rette e mense scolastiche (FSE+ Abruzzo 2021-2027) |

## Licenza

[EUPL 1.2](LICENSE) — Licenza Pubblica dell'Unione europea
