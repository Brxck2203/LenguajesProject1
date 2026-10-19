# GameStats — paquete de instalación

Este paquete utiliza las imágenes publicadas en GitHub Container Registry. Requiere Docker con Compose v2 iniciado. Ejecutar los comandos desde la carpeta que contiene `docker-compose.release.yml`.

## Instalar

Windows (PowerShell):

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -Release
```

Linux/macOS/WSL:

```sh
bash scripts/install.sh --release
```

El instalador descarga las imágenes de esta versión, levanta los servicios y ejecuta la prueba de integración. Debe terminar indicando `OK: Go y Scala integrados`. Los paquetes del registro deben ser públicos; si son privados, iniciar sesión con `docker login ghcr.io` y un token con permiso `read:packages`.

## Usar y detener

En este paquete, todos los comandos Compose deben seleccionar explícitamente el archivo Release:

```sh
docker compose -f docker-compose.release.yml ps
docker compose -f docker-compose.release.yml run --rm simulator -scenario random -matches 5 -wait
docker compose -f docker-compose.release.yml logs --tail=100 go-service scala-analytics
docker compose -f docker-compose.release.yml down
docker compose -f docker-compose.release.yml up -d --wait
```

Consultar <http://localhost:8081/matches>, <http://localhost:8080/results> y <http://localhost:8080/stats/games>. Las respuestas se muestran en JSON. Los resultados se guardan en `data/results` y sobreviven a la recreación de contenedores; el estado de Go es temporal.

Este paquete no incluye el código fuente ni el Compose de desarrollo. La documentación técnica está en `docs/`; los pasos que construyen desde fuentes corresponden al repositorio completo.
