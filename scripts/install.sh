#!/usr/bin/env bash
# Instala y levanta GameStats en una máquina con Docker (Linux, macOS o WSL).
#   bash scripts/install.sh          construye las imágenes desde el código fuente
#   ./scripts/install.sh --release  usa las imágenes publicadas por el pipeline (docker-compose.release.yml)
#   ./scripts/install.sh --no-test  no ejecuta la prueba de humo al final
set -euo pipefail
cd "$(dirname "$0")/.."

RELEASE=false
RUN_SMOKE=true
for arg in "$@"; do
  case "$arg" in
    --release) RELEASE=true ;;
    --no-test) RUN_SMOKE=false ;;
    *) echo "opción desconocida: $arg" >&2; exit 2 ;;
  esac
done

echo "==> Verificando requisitos"
command -v docker >/dev/null || { echo "Docker no está instalado: https://docs.docker.com/get-docker/" >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo "Docker no está corriendo (abre Docker Desktop o inicia el servicio)" >&2; exit 1; }
docker compose version >/dev/null || { echo "Falta Docker Compose v2" >&2; exit 1; }

echo "==> Preparando carpetas de datos"
# Los contenedores corren con usuarios sin privilegios: las carpetas deben admitir escritura.
mkdir -p data/inbox/processed data/inbox/rejected data/results
chmod -R a+rwX data

echo "==> Levantando servicios"
if $RELEASE; then
  # Docker Compose lee COMPOSE_FILE; la prueba de humo hereda la misma configuración.
  export COMPOSE_FILE=docker-compose.release.yml
  docker compose pull
  docker compose up -d --wait
else
  docker compose up -d --build --wait
fi
docker compose ps

if $RUN_SMOKE; then
  echo "==> Prueba de humo"
  bash ./scripts/smoke-test.sh
fi

cat <<'MSG'

GameStats está listo:
  Panel de revisión:  http://localhost:8081/dashboard
  Go (ingestión):      http://localhost:8081/        (partidas: /matches, catálogo: /catalog, bandeja: /inbox)
  Scala (análisis):    http://localhost:8080/        (resultados: /results, por juego: /stats/games)
  Bandeja de eventos:  ./data/inbox    Resultados JSON: ./data/results

Generar partidas:      docker compose run --rm simulator -scenario random -matches 5 -wait
Fuente continua:       docker compose --profile live up -d live-simulator
Detener:               docker compose down
MSG
