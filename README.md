# GameStats

Sistema de procesamiento y análisis de eventos de partidas de **MMA, fútbol, motociclismo y Free Fire**.

Go recibe los eventos por HTTP o archivos, los valida, organiza las partidas y administra su procesamiento concurrente. Scala calcula estadísticas por jugador y por partida, detecta patrones y guarda los resultados. El panel web permite consultar la información y generar partidas de prueba.

## Requisitos

- Docker con Docker Compose v2. En Windows y macOS, iniciar Docker Desktop con contenedores Linux.
- PowerShell en Windows, o Bash en Linux/macOS/WSL.
- Internet para descargar las imágenes y dependencias durante la primera instalación.

No se necesita instalar Go, Scala, Java, sbt ni herramientas de frontend para ejecutar el proyecto con Docker. Git solo es necesario si se obtiene el proyecto mediante un clon; también se puede descargar el código como ZIP y descomprimirlo.

## Ejecutar desde el código fuente

Obtener la versión de `main`:

```sh
git clone https://github.com/Brxck2203/LenguajesProject1.git
cd LenguajesProject1
```

Si ya se tiene el repositorio:

```sh
git switch main
git pull --ff-only
```

Abrir una terminal en la carpeta que contiene `docker-compose.yml`. Los puertos 8080 y 8081 deben estar disponibles.

**Windows, desde PowerShell:**

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install.ps1
```

**Linux, macOS o WSL:**

```sh
bash scripts/install.sh
```

El instalador prepara las carpetas de datos, construye y prueba ambos módulos, levanta los servicios y ejecuta una prueba integrada con eventos por HTTP y archivos. La primera construcción puede tardar varios minutos.

Al finalizar debe aparecer `OK: Go y Scala integrados`. Confirmar el estado:

```sh
docker compose ps
```

Los servicios `go-service` y `scala-analytics` deben aparecer `healthy`.

## Usar el panel

Abrir **<http://localhost:8081/dashboard>**.

| Sección | Contenido |
|---|---|
| Resumen | Partidas analizadas, eventos procesados, estadísticas por juego y patrones detectados. |
| Partidas y patrones | Búsqueda de partidas, estadísticas individuales, resumen de partida y eventos que sustentan cada patrón. |
| Jugadores | Historial por juego, victorias y métricas acumuladas por jugador. |
| Entrada de eventos | Estado de la bandeja de archivos y catálogo de acciones admitidas. |

El botón **Generar demostración** crea ocho partidas nuevas, dos por juego, con 73 eventos. Esperar unos segundos para que Scala termine los análisis. El panel actualiza los datos automáticamente; también se puede pulsar **Actualizar**.

En el detalle, `COMPLETED` indica que el análisis terminó. Si aparece `FAILED`, revisar el error y utilizar **Reintentar análisis** después de resolver su causa. Una partida puede terminar correctamente sin detectar patrones.

## Generar más datos

Partidas aleatorias por HTTP:

```sh
docker compose run --rm simulator -scenario random -matches 5 -wait
```

Genera **20 partidas: cinco por cada juego**. `-wait` espera a que finalicen los análisis y termina con error si alguno falla.

Partidas mediante archivos:

```sh
docker compose run --rm simulator -mode files -scenario random -matches 2 -wait
```

Fuente continua de eventos:

```sh
docker compose --profile live up -d live-simulator
```

Detener únicamente esa fuente:

```sh
docker compose --profile live stop live-simulator
```

El simulador utiliza identificadores nuevos en cada ejecución. Si se proporciona `-prefix`, cambiarlo para cada grupo de partidas distintas. Para reproducir escenarios aleatorios, utilizar `-seed`, por ejemplo `-seed 42`.

## Consultar las API

Las respuestas JSON continúan disponibles para consultas técnicas:

| Servicio | Ruta | Contenido |
|---|---|---|
| Go | `GET /health` | Salud del servidor Go. |
| Go | `GET /matches` | Partidas recibidas en el proceso actual. |
| Go | `GET /matches/{matchId}` | Estado de la partida, participantes y cantidad de eventos. |
| Go | `GET /matches/{matchId}/analysis` | Estado del análisis y resultado, si está disponible. |
| Go | `POST /matches/{matchId}/analysis/retry` | Reintentar un análisis fallido. |
| Go | `POST /events` | Recibir un evento. |
| Go | `POST /events/batch` | Recibir un grupo de eventos. |
| Go | `GET /catalog` | Formato de eventos y reglas de validación. |
| Go | `GET /inbox` | Estado de la bandeja de archivos. |
| Scala | `GET /health` | Salud del servidor Scala. |
| Scala | `GET /results` | Resultados guardados. |
| Scala | `GET /results/{matchId}` | Análisis completo de una partida. |
| Scala | `GET /stats/games` | Estadísticas agregadas por juego. |
| Scala | `GET /stats/games/{gameId}/players` | Historial de jugadores de un juego. |
| Scala | `POST /analyze` | Recibir la partida preparada por Go y devolver el análisis. |

Go escucha en <http://localhost:8081> y Scala en <http://localhost:8080>. La raíz de cada servicio ofrece un índice JSON; la interfaz visual se encuentra en `/dashboard` del servidor Go.

Ejemplo en PowerShell:

```powershell
Invoke-RestMethod http://localhost:8080/stats/games | ConvertTo-Json -Depth 20
```

## Datos y persistencia

| Carpeta | Uso |
|---|---|
| `data/inbox/` | Archivos de eventos pendientes de procesar. |
| `data/inbox/processed/` | Archivos procesados. |
| `data/inbox/rejected/` | Archivos rechazados y reportes `*.error.json`. |
| `data/results/` | Resultados JSON guardados por Scala. |

Scala recupera los resultados al iniciar e ignora archivos inválidos informando el error en sus registros. Los resultados guardados continúan consultables desde el panel y `/results` después de reiniciar los contenedores.

El estado de Go es temporal: al reiniciarlo se pierden las partidas, identificadores y trabajos de análisis que conservaba en memoria. Esto no borra los resultados ya guardados por Scala.

## Administrar los servicios

```sh
# Consultar los registros
docker compose logs --tail=150 go-service scala-analytics

# Ver registros mientras se generan eventos
docker compose logs -f go-service scala-analytics

# Detener y retirar los contenedores; los archivos de data/ permanecen
docker compose down

# Volver a iniciar con las imágenes construidas
docker compose up -d --wait

# Reconstruir después de actualizar el código
docker compose up -d --build --wait
```

Para cambiar puertos, copiar `.env.example` como `.env` y ajustar `GO_PORT` y `SCALA_PORT`. Usar los nuevos puertos al abrir el panel o consultar las API. Si la instalación falla, revisar primero `docker compose ps` y los registros.

## Ejecutar con imágenes publicadas

Esta opción descarga las imágenes ya construidas; no compila el proyecto. Requiere que las imágenes estén publicadas en GHCR y sean públicas, o haber iniciado sesión mediante `docker login ghcr.io`.

Desde el repositorio o el paquete de instalación de una Release:

```powershell
# Windows
powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -Release
```

```sh
# Linux/macOS/WSL
bash scripts/install.sh --release
```

Para administrar esta instalación, seleccionar el archivo Release:

```sh
docker compose -f docker-compose.release.yml ps
docker compose -f docker-compose.release.yml run --rm simulator -scenario random -matches 5 -wait
docker compose -f docker-compose.release.yml down
```

El paquete de una Release contiene Compose, scripts e instrucciones. El repositorio conserva el código fuente completo.

## Pruebas y CI/CD

El instalador ejecuta las pruebas dentro de las imágenes y una prueba integrada después de iniciar los servicios. Para desarrollo sin Docker se necesitan Go 1.22+, Java 17 y sbt 1.9.7.

```sh
# En go_service/
go vet ./...
go test -race ./...

# En scala_service/
sbt test assembly
```

El workflow [CI/CD](.github/workflows/ci-cd.yml) compila y prueba ambos módulos y verifica la instalación integrada en Docker. Los cambios en `main` publican las imágenes en GHCR y verifican su ejecución. Las etiquetas `v*` generan además el paquete Release.

Los runners hospedados de GitHub son temporales; esa verificación no deja un servidor público permanente. El proyecto se ejecuta en el equipo donde se inicie Docker Compose.

## Documentación

- [Instalación y prueba local con Docker Desktop](docs/docker-local.md).
- [Despliegue y publicación de versiones](docs/despliegue.md).
- [Formato de eventos](docs/eventos.md).
- [Contrato entre Go y Scala](docs/contrato-go-scala.md).
- [Juegos, estadísticas y patrones](docs/juegos-y-estadisticas.md).
- [Arquitectura](docs/arquitectura.md).
- [Módulo Go](go_service/README.md).
- [Módulo Scala](scala_service/README.md).
