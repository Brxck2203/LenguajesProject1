# Instala y levanta GameStats en Windows con Docker Desktop.
#   powershell -ExecutionPolicy Bypass -File scripts\install.ps1            (construye desde el código)
#   powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -Release   (usa imágenes publicadas)
#   powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -NoTest
param([switch]$Release, [switch]$NoTest)
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')

Write-Host '==> Verificando requisitos'
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Docker no está instalado: https://docs.docker.com/desktop/' }
docker info *> $null
if ($LASTEXITCODE -ne 0) { throw 'Docker Desktop no está corriendo' }

Write-Host '==> Preparando carpetas de datos'
foreach ($dir in 'data\inbox\processed', 'data\inbox\rejected', 'data\results') {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
}

Write-Host '==> Levantando servicios'
if ($Release) {
    $env:COMPOSE_FILE = 'docker-compose.release.yml'
    docker compose pull
    if ($LASTEXITCODE -ne 0) { throw 'No se pudieron descargar las imágenes' }
    docker compose up -d --wait
} else {
    docker compose up -d --build --wait
}
if ($LASTEXITCODE -ne 0) { throw 'docker compose up falló; revisa: docker compose logs' }
docker compose ps

if (-not $NoTest) {
    Write-Host '==> Prueba de humo'
    $stamp = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    docker compose run --rm -T simulator -scenario all -matches 1 -interval 2ms -wait -timeout 3m -prefix "smoke_http_$stamp"
    if ($LASTEXITCODE -ne 0) { throw 'La simulación por HTTP falló' }
    docker compose run --rm -T simulator -mode files -scenario random -matches 1 -interval 20ms -wait -timeout 3m -prefix "smoke_files_$stamp"
    if ($LASTEXITCODE -ne 0) { throw 'La simulación por archivos falló' }
    $scalaBinding = docker compose port scala-analytics 8080
    if ($LASTEXITCODE -ne 0 -or -not $scalaBinding) { throw 'No se pudo consultar el puerto de Scala' }
    $scalaPort = (($scalaBinding | Select-Object -First 1).Trim() -split ':')[-1]
    $summary = Invoke-RestMethod "http://localhost:$scalaPort/stats/games"
    foreach ($game in 'MMA', 'FOOTBALL', 'MOTORCYCLE_RACING', 'FREE_FIRE') {
        if (-not $summary.$game) { throw "No hay estadísticas de $game" }
    }
    $summaryJson = $summary | ConvertTo-Json -Depth 10
    foreach ($pattern in 'FLURRY_KNOCKDOWN', 'TEAM_BUILDUP_GOAL', 'COMEBACK_AFTER_INCIDENT', 'THIRD_PARTY') {
        if ($summaryJson -notmatch "`"$pattern`"") { throw "No se detectó el patrón $pattern" }
    }
    if (-not (Get-ChildItem "data\results\smoke_http_${stamp}_*.json" -ErrorAction SilentlyContinue)) {
        throw 'Scala no guardó resultados en data\results'
    }
    Write-Host 'OK: Go y Scala integrados; resultados en data\results'
}

Write-Host ''
Write-Host 'GameStats está listo:'
Write-Host '  Go (ingestión):    http://localhost:8081/   (/matches, /catalog, /inbox)'
Write-Host '  Scala (análisis):  http://localhost:8080/   (/results, /stats/games)'
Write-Host '  Bandeja: .\data\inbox   Resultados: .\data\results'
Write-Host '  Generar partidas:  docker compose run --rm simulator -scenario random -matches 5 -wait'
Write-Host '  Detener:           docker compose down'
