# Deploy — Production Compose + Backup Design Spec

**Data:** 2026-06-15  
**Scope:** Convertire `docker-compose.yml` in configurazione production-ready per Portainer Stack; aggiungere sidecar backup SQLite; separare build locale in `docker-compose.override.yml`

---

## Obiettivo

Permettere il deploy su Proxmox/Podman tramite Portainer Stack senza accesso shell. Il compose di produzione usa l'immagine pre-buildata da GHCR (CI/CD già funzionante). Il backup SQLite è un sidecar nello stack, con dati accessibili via Portainer UI → Volumes → Export.

---

## Architettura

### Pattern base + override

Docker Compose carica automaticamente `docker-compose.override.yml` in locale. Portainer usa solo `docker-compose.yml`.

| File | Usato da | Contiene |
|------|----------|---------|
| `docker-compose.yml` | Portainer (prod) + locale (con override) | Immagine GHCR, backup sidecar, volumi |
| `docker-compose.override.yml` | Solo locale (`docker compose up`) | `build: .`, backup in profile disabilitato |
| `deploy/stack.env.example` | Documentazione Portainer | Template variabili d'ambiente |

### Flusso deploy prod

1. CI/CD pusha `ghcr.io/comune-di-montesilvano/opencity-gestionale:latest` (già funzionante)
2. In Portainer: Stacks → Add Stack → incolla contenuto `docker-compose.yml` + configura env vars
3. Per aggiornare: Portainer → Stack → Pull and redeploy

---

## `docker-compose.yml` (produzione)

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

**Note:**
- `$$` nei command YAML — escape del `$` per evitare interpolazione da parte di Compose
- `apk add sqlite` ad ogni restart: Alpine 3.21, pacchetto < 1MB, avvio ~2s, accettabile
- `gestionale_backup` volume accessibile via Portainer UI per export manuale
- `env_file` rimosso: Portainer gestisce le variabili tramite la UI "Environment variables" dello stack
- `OPENCITY_BASE_URL` portato a variabile (era hardcoded nel compose originale) per flessibilità

---

## `docker-compose.override.yml` (sviluppo locale)

```yaml
services:
  gestionale:
    build: .
    image: ""
  backup:
    profiles:
      - backup
```

**Note:**
- `build: .` sovrascrive `image:` per costruire localmente
- `image: ""` necessario per evitare che Compose cerchi di pullare l'immagine GHCR in locale
- `profiles: [backup]` disabilita il backup sidecar in locale (non parte con `docker compose up`)
- Per avviare il backup anche in locale: `docker compose --profile backup up`

---

## `deploy/stack.env.example`

Template da copiare in Portainer come variabili d'ambiente dello stack:

```env
SECRET_KEY=cambia-con-almeno-32-caratteri-casuali
ADMIN_USERNAMES=apioperator
OPENCITY_BASE_URL=https://service.comune.montesilvano.pe.it
PORT=8080
```

---

## File da modificare/creare

| File | Operazione |
|------|------------|
| `docker-compose.yml` | Modifica: `build` → `image` GHCR, aggiunge servizio `backup`, rimuove `env_file`, porta `OPENCITY_BASE_URL` a variabile |
| `docker-compose.override.yml` | Crea: `build: .` + backup in profile |
| `deploy/stack.env.example` | Crea: template variabili Portainer |
| `TODO.md` | Modifica: segna deploy items come completati |

---

## Fuori scope

- Configurazione TLS/reverse proxy (esterno)
- Litestream o replica continua SQLite
- Alerting su fallimento backup
- Rotazione automatica su storage remoto
