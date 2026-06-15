# Deploy — Production Compose + Backup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convertire `docker-compose.yml` in configurazione production-ready per Portainer, aggiungere sidecar backup SQLite giornaliero, separare build locale in `docker-compose.override.yml`.

**Architecture:** `docker-compose.yml` diventa il file prod (immagine GHCR, backup sidecar). `docker-compose.override.yml` sovrascrive per lo sviluppo locale (`build: .`, backup disabilitato via profile). Portainer usa solo il file base.

**Tech Stack:** Docker Compose v2, Alpine 3.21 (backup), SQLite3, GHCR

---

## File map

| File | Operazione | Responsabilità |
|------|------------|----------------|
| `docker-compose.yml` | Modifica | Prod: immagine GHCR + backup sidecar + volumi |
| `docker-compose.override.yml` | Crea | Dev locale: `build: .` + backup in profile |
| `deploy/stack.env.example` | Crea | Template variabili d'ambiente per Portainer |
| `TODO.md` | Modifica | Segna i 3 deploy items come completati |

---

## Task 1: Aggiorna `docker-compose.yml` per produzione

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Sostituisci l'intero contenuto di `docker-compose.yml`**

```yaml
services:
  gestionale:
    image: ghcr.io/comune-di-montesilvano/opencity-gestionale:latest
    ports:
      - "127.0.0.1:${PORT:-8080}:${PORT:-8080}"
    volumes:
      - gestionale_data:/data
    environment:
      PORT: ${PORT:-8080}
      DB_PATH: /data/gestionale.db
      OPENCITY_BASE_URL: ${OPENCITY_BASE_URL}
      SECRET_KEY: ${SECRET_KEY}
      ADMIN_USERNAMES: ${ADMIN_USERNAMES:-apioperator}
      TRUST_PROXY: "true"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:${PORT:-8080}/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    restart: unless-stopped

  backup:
    image: alpine:3.21
    volumes:
      - gestionale_data:/data:ro
      - gestionale_backup:/backup
    entrypoint: ["/bin/sh", "-c"]
    command:
      - |
        apk add --no-cache sqlite > /dev/null 2>&1
        while true; do
          FNAME="/backup/gestionale-$$(date +%Y%m%d-%H%M).db"
          sqlite3 /data/gestionale.db ".backup $$FNAME"
          find /backup -name '*.db' -mtime +30 -delete
          sleep 86400
        done
    restart: unless-stopped
    depends_on:
      gestionale:
        condition: service_healthy

volumes:
  gestionale_data:
  gestionale_backup:
```

**Note implementative:**
- `build:` e `env_file:` rimossi — Portainer non ha source code e gestisce le variabili via UI
- `OPENCITY_BASE_URL` ora variabile (era hardcoded `https://service.comune.montesilvano.pe.it`)
- `TRUST_PROXY: "true"` fisso — in produzione c'è sempre un reverse proxy esterno
- `$$` nel command YAML: escape del `$` per evitare interpolazione da parte di Compose (il `$` deve arrivare alla shell come `$`)
- `depends_on: condition: service_healthy` — il backup parte solo dopo che il gestionale è healthy

- [ ] **Step 2: Valida la sintassi**

```bash
docker compose config
```

Output atteso: YAML valido stampato a schermo, nessun errore. Se Docker non è disponibile localmente, salta questo step — Portainer lo validerà al deploy.

- [ ] **Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "feat(deploy): production compose with GHCR image and SQLite backup sidecar"
```

---

## Task 2: Crea `docker-compose.override.yml` per sviluppo locale

**Files:**
- Create: `docker-compose.override.yml`

- [ ] **Step 1: Crea il file**

```yaml
# Sviluppo locale — caricato automaticamente da `docker compose up`
# Portainer ignora questo file (non è nel repository Portainer)
services:
  gestionale:
    build: .
    image: ""
  backup:
    profiles:
      - backup
```

**Note implementative:**
- `build: .` sovrascrive `image:` del file base
- `image: ""` necessario: senza di esso Compose tenta di pullare l'immagine GHCR anche con `build:` presente
- `profiles: [backup]` esclude il sidecar backup da `docker compose up` locale (troppo lento per dev)
- Per avviare il backup anche in locale: `docker compose --profile backup up`

- [ ] **Step 2: Verifica che il merge locale sia corretto**

```bash
docker compose config --no-interpolate 2>&1 | head -20
```

Output atteso: il servizio `gestionale` deve mostrare `build: {context: .}` e NON `image: ghcr.io/...`. Il servizio `backup` deve mostrare `profiles: [backup]`.

- [ ] **Step 3: Commit**

```bash
git add docker-compose.override.yml
git commit -m "feat(deploy): add override for local dev build"
```

---

## Task 3: Crea `deploy/stack.env.example`

**Files:**
- Create: `deploy/stack.env.example`

- [ ] **Step 1: Crea la directory e il file**

```bash
mkdir -p deploy
```

Crea `deploy/stack.env.example` con:

```env
# Variabili d'ambiente per Portainer Stack
# Copia questi valori nella sezione "Environment variables" dello stack in Portainer
# NON committare il file con i valori reali

# Obbligatori
SECRET_KEY=cambia-con-almeno-32-caratteri-casuali-es-openssl-rand-hex-32
OPENCITY_BASE_URL=https://service.comune.montesilvano.pe.it

# Opzionali (valori di default mostrati)
ADMIN_USERNAMES=apioperator
PORT=8080
```

- [ ] **Step 2: Verifica che `.gitignore` non escluda il file example**

```bash
git check-ignore -v deploy/stack.env.example
```

Output atteso: nessun output (= file non ignorato). Se viene ignorato, aggiungi `!deploy/stack.env.example` al `.gitignore`.

- [ ] **Step 3: Commit**

```bash
git add deploy/stack.env.example
git commit -m "docs(deploy): add Portainer stack env template"
```

---

## Task 4: Aggiorna `TODO.md`

**Files:**
- Modify: `TODO.md`

- [ ] **Step 1: Segna i deploy items come completati**

In `TODO.md`, sezione `## Deploy`, cambia i tre item da `- [ ]` a `- [x]` e aggiungi nota:

```markdown
## Deploy

- [x] **Proxmox Podman**: deploy via Portainer Stack — `docker-compose.yml` usa immagine GHCR, `docker-compose.override.yml` per build locale
- [x] **Reverse proxy TLS**: esterno, fuori scope — impostare `TRUST_PROXY=true` già hardcoded in prod compose
- [x] **Backup SQLite**: sidecar `backup` nello stack — Alpine + sqlite3, loop giornaliero, pruning 30gg, volume `gestionale_backup` exportabile da Portainer UI
```

- [ ] **Step 2: Commit**

```bash
git add TODO.md
git commit -m "chore: mark deploy items as done in TODO"
```

---

## Self-review

**Spec coverage:**
- ✅ `docker-compose.yml` → prod con GHCR image (Task 1)
- ✅ Backup sidecar alpine + sqlite3 + pruning 30gg (Task 1)
- ✅ `docker-compose.override.yml` con `build: .` + backup in profile (Task 2)
- ✅ `deploy/stack.env.example` template Portainer (Task 3)
- ✅ TODO aggiornato (Task 4)

**Placeholder scan:** Nessun TBD. Tutti i contenuti file sono completi e copy-paste ready.

**Type consistency:** N/A — nessun codice Go, solo YAML e config.

**Nota `$$`:** Il double-dollar `$$` nel `command` YAML è l'escape corretto per Docker Compose. La shell riceve `$FNAME` e `$(date ...)` come variabili/command substitution normali. Senza escape, Compose interpolerebbe `$FNAME` come variabile d'ambiente vuota.
