#!/bin/sh
# Deploy manuale sul server.
# Uso: ./deploy.sh
# Prerequisiti: .env presente nella stessa directory, docker compose disponibile.
set -e

echo "Pulling nuova immagine..."
docker compose pull

echo "Riavvio container..."
docker compose up -d

echo "Deploy completato."
docker compose ps
