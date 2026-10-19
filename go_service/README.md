# GameStats — módulo Go (ingestión y control del flujo)

Recibe eventos simulados o de la consola de juegos por HTTP o por archivos, los valida contra el catálogo, reconstruye las partidas en memoria y envía cada partida cerrada a Scala para analizarla. **Go no calcula estadísticas**: transporta el lote y guarda el resultado que devuelve Scala.

## Recorrido de una partida

1. Un productor envía `MATCH_STARTED`, las acciones y `MATCH_FINISHED`, por `POST /events`, `POST /events/batch` o como archivos en `INBOX_DIR`.
2. Go valida cada evento con el catálogo (`internal/validation/event_catalog.json`), rechaza duplicados y eventos fuera del ciclo de vida, y los agrupa por `matchId`.
3. Al cerrar la partida, crea una copia ordenada por `timestamp` y registra el trabajo de análisis en una cola con capacidad limitada.
4. Un worker envía el lote a `SCALA_ANALYSIS_URL` (`POST /analyze`) y reintenta los fallos transitorios.
5. Go verifica la identidad y la estructura del resultado y lo deja disponible en `/matches/{id}/analysis`.

## Concurrencia

| Componente | Mecanismo |
|---|---|
| API HTTP | `net/http` atiende cada petición en su goroutine; el estado de las partidas usa `sync.RWMutex` |
| Bandeja de archivos (`internal/inbox`) | Un goroutine *dispatcher* lee la carpeta en orden y separa cada archivo por `matchId`; N workers con un channel cada uno. La partición por hash del `matchId` mantiene el orden dentro de cada partida y procesa partidas distintas en paralelo |
| Análisis (`internal/processing`) | Channel con capacidad `ANALYSIS_CAPACITY` y `ANALYSIS_WORKERS` goroutines; las llamadas de red ocurren fuera de los bloqueos |
| Simulador (`internal/simulation`) | Una goroutine por partida; partidas de los cuatro juegos avanzan a la vez |
| Apagado | SIGINT/SIGTERM: primero se deja de leer la bandeja, luego se cierra HTTP y se vacía la cola de análisis |

## Estructura

| Ruta | Responsabilidad |
|---|---|
| `cmd/server` | Configuración, servidor HTTP, bandeja y apagado controlado |
| `cmd/simulator` | Fuente de eventos simulada (HTTP o archivos; guionada o aleatoria; continua) |
| `internal/domain` | Sobre de eventos, lotes, estados y resultados |
| `internal/validation` | Catálogo de eventos (JSON embebido o `EVENT_CATALOG_PATH`) y validador |
| `internal/match` | Estado temporal, duplicados, participantes y ciclo de vida |
| `internal/processing` | Cierre atómico, cola, workers y reintentos |
| `internal/inbox` | Ingestión concurrente de archivos `*.json`, `*.jsonl` y `*.ndjson` |
| `internal/scala` | Cliente HTTP del contrato con Scala |
| `internal/simulation` | Partidas guionadas, generador aleatorio y productores HTTP y de archivos |
| `internal/transport/http` | Rutas, códigos HTTP y respuestas JSON |
| `internal/config` | Lectura y validación de variables de entorno |

## API HTTP

| Método | Ruta | Respuesta |
|---|---|---|
| GET | `/` | Índice de rutas y juegos del catálogo |
| GET | `/health` | Salud del proceso Go (no verifica Scala) |
| GET | `/catalog` | Catálogo de eventos vigente |
| GET | `/inbox` | Estado de la bandeja de archivos |
| POST | `/events` | 202: evento aceptado |
| POST | `/events/batch` | 202: lote `{"events":[...]}` aceptado; si un evento falla se conserva lo anterior y se indica `failedAtIndex` |
| GET | `/matches` | Partidas con enlaces a resumen y análisis |
| GET | `/matches/{matchId}` | Resumen: juego, participantes, cantidad de eventos, estado |
| GET | `/matches/{matchId}/analysis` | Estado del análisis y resultado |
| POST | `/matches/{matchId}/analysis/retry` | 202: nuevo ciclo para un análisis `FAILED` |

| Código | Casos |
|---|---|
| 400 | JSON inválido o evento que no cumple el catálogo |
| 404 | Partida o ruta inexistente |
| 405 | Método incorrecto (incluye `Allow`) |
| 409 | Duplicado, conflicto con el ciclo de vida, el juego o el tiempo; análisis no reintentable |
| 413 | Cuerpo mayor de 1 MiB |
| 415 | `Content-Type` distinto de `application/json` |
| 503 | Capacidad de análisis agotada o servicio cerrándose (`Retry-After: 1`) |

| Estado del análisis | Significado |
|---|---|
| `QUEUED` | En cola |
| `PROCESSING` | Petición a Scala en curso |
| `RETRYING` | Esperando el siguiente intento automático |
| `COMPLETED` | Resultado recibido de Scala |
| `FAILED` | Error permanente o intentos agotados; se puede reintentar manualmente |

## Simulador

```sh
go run ./cmd/simulator                                   # 8 partidas guionadas por HTTP
go run ./cmd/simulator -scenario random -matches 10 -wait
go run ./cmd/simulator -mode files -out ../data/inbox -scenario all
go run ./cmd/simulator -mode files -out ../data/inbox -scenario random -matches 1 -continuous -every 20s -timeout 0
```

| Partida guionada | Variante 1 | Variante 2 |
|---|---|---|
| MMA | Tres golpes seguidos, knockdown y detención | Golpes separados, sin knockdown |
| FOOTBALL | Cinco pases completados del mismo equipo y gol | Secuencia cortada por una falta y un pase fallado |
| MOTORCYCLE_RACING | Caída y recuperación hasta el primer lugar | Parada en boxes sin recuperación |
| FREE_FIRE | A daña a B y C elimina a B (*third party*) | El mismo atacante daña y elimina |

El generador aleatorio (`-scenario random`) es reproducible con `-seed` y reutiliza grupos fijos de jugadores, así cada uno acumula historial entre partidas. Detalles en [docs/juegos-y-estadisticas.md](../docs/juegos-y-estadisticas.md#5-datos-de-prueba).

## Configuración

| Variable | Por defecto | Uso |
|---|---|---|
| `PORT` | `8081` | Puerto HTTP |
| `SCALA_ANALYSIS_URL` | `http://localhost:8080/analyze` | Endpoint de Scala |
| `SCALA_TIMEOUT` | `10s` | Tiempo máximo por petición a Scala |
| `ANALYSIS_WORKERS` | `4` | Workers de análisis |
| `ANALYSIS_CAPACITY` | `128` | Análisis activos como máximo |
| `ANALYSIS_MAX_ATTEMPTS` | `3` | Intentos por ciclo |
| `ANALYSIS_RETRY_BASE` / `ANALYSIS_RETRY_MAX` | `1s` / `10s` | Espera exponencial entre intentos |
| `SHUTDOWN_TIMEOUT` | `30s` | Plazo para el apagado controlado |
| `EVENT_CATALOG_PATH` | (vacío) | Catálogo alternativo; vacío usa el embebido |
| `INBOX_DIR` | (vacío) | Carpeta de la bandeja; vacío la desactiva (Docker usa `/data/inbox`) |
| `INBOX_POLL_INTERVAL` | `500ms` | Frecuencia de revisión de la carpeta |
| `INBOX_WORKERS` | `4` | Workers de la bandeja |

## Verificación

```sh
gofmt -l .
go vet ./...
go test -race ./...
```

Las pruebas cubren la validación y el catálogo (incluido un juego nuevo definido solo por JSON), el estado de las partidas, la cola con reintentos y capacidad llena, el cliente de Scala, la API HTTP con un Scala simulado, el orden y el paralelismo de la bandeja, archivos rechazados con su reporte, y que todas las partidas generadas, guionadas y aleatorias, sean aceptadas.

## Garantías y límites

- `eventId` es único durante la vida del proceso. Una partida solo abre con `MATCH_STARTED`; después del cierre no se aceptan más eventos.
- Con la capacidad llena, el cierre se rechaza **antes** de modificar la partida, para que el productor pueda reenviarlo.
- El estado de Go está en memoria: reiniciar el proceso borra las partidas en curso. El historial persistente lo guarda Scala en `data/results`.
