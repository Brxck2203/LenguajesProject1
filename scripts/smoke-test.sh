#!/usr/bin/env bash
# Prueba de humo de la instalación: genera partidas por HTTP y por archivos, espera los
# análisis de Scala y comprueba las estadísticas de los cuatro juegos.
set -euo pipefail
cd "$(dirname "$0")/.."
# Read the effective published ports, including values provided through .env.
compose_port() {
  docker compose port "$1" "$2" | awk -F: 'NR == 1 {print $NF}'
}
GO_URL="${GO_URL:-http://localhost:$(compose_port go-service 8081)}"
SCALA_URL="${SCALA_URL:-http://localhost:$(compose_port scala-analytics 8080)}"
STAMP="$(date +%s)"

wait_for() {
  for _ in $(seq 1 60); do
    curl -fsS "$1" >/dev/null 2>&1 && return 0
    sleep 2
  done
  echo "FALLO: $1 no responde" >&2
  return 1
}

wait_for "$GO_URL/health"
wait_for "$SCALA_URL/health"

echo "-- 8 partidas guionadas y 4 aleatorias por HTTP"
docker compose run --rm -T simulator -scenario all -matches 1 -interval 2ms -wait -timeout 3m -prefix "smoke_http_$STAMP"

echo "-- 4 partidas aleatorias como archivos en la bandeja"
docker compose run --rm -T simulator -mode files -scenario random -matches 1 -interval 20ms -wait -timeout 3m -prefix "smoke_files_$STAMP"

summary="$(curl -fsS "$SCALA_URL/stats/games")"
for game in MMA FOOTBALL MOTORCYCLE_RACING FREE_FIRE; do
  grep -q "\"$game\"" <<< "$summary" || { echo "FALLO: no hay estadísticas de $game" >&2; exit 1; }
done
for pattern in FLURRY_KNOCKDOWN TEAM_BUILDUP_GOAL COMEBACK_AFTER_INCIDENT THIRD_PARTY; do
  grep -q "\"$pattern\"" <<< "$summary" || { echo "FALLO: no se detectó el patrón $pattern" >&2; exit 1; }
done
ls data/results/smoke_http_"$STAMP"_*.json >/dev/null 2>&1 || { echo "FALLO: Scala no guardó resultados en data/results" >&2; exit 1; }

curl -fsS "$GO_URL/dashboard" -o /tmp/gamestats-dashboard.html
grep -q '<title>GameStats' /tmp/gamestats-dashboard.html
curl -fsS "$GO_URL/dashboard/api/health" >/dev/null

echo "OK: Go y Scala integrados; panel en $GO_URL/dashboard; resultados en data/results"
