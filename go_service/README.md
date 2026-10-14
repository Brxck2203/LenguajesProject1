# GameStats — Servicio Go

Go recibe eventos simulados, valida su contrato, reconstruye partidas en memoria y envía cada partida cerrada a Scala para su análisis. Las estadísticas y los patrones se calculan exclusivamente en Scala.

## Estado del módulo

| Componente | Estado |
|---|---|
| API HTTP de ingestión individual y batch | Implementada |
| Validación y estado de partidas en memoria | Implementados |
| Simulación concurrente de cuatro juegos | Implementada: ocho partidas por ejecución |
| Cola, workers y reintentos de análisis | Implementados |
| Cliente HTTP y verificación de respuesta Scala | Implementados |
| Consulta de análisis y reintento manual | Implementados |
| Apagado controlado por SIGINT/SIGTERM | Implementado |
| Dockerfile propio de Go | Incluido; ejecución Docker pendiente de verificar |
| Integración con el Scala actual del repositorio | Pendiente de adaptar Scala al contrato |

**Go funciona sin Scala para recibir y consultar partidas.** En ese caso, el análisis quedará en `RETRYING` y finalmente `FAILED`. Esto no impide consultar la partida ni conserva falsos resultados. Una vez disponible un Scala compatible, se pueden reintentar los lotes fallidos sin reabrir las partidas, mientras el proceso Go siga vivo.

La definición vigente para el próximo trabajo de Scala está en [el contrato de integración](docs/scala_contract.md). El README raíz describe otra propuesta de integración y no sustituye las instrucciones de este módulo.

## Recorrido de una partida

1. Un productor envía `MATCH_STARTED`, acciones y `MATCH_FINISHED` a Go.
2. Go valida y agrupa por `matchId`; el historial conserva el orden de llegada.
3. El cierre crea una copia ordenada establemente por timestamp y registra el trabajo de análisis.
4. Un worker envía el lote mediante `POST` a `SCALA_ANALYSIS_URL`.
5. Go valida la identidad y estructura del resultado y lo mantiene en memoria.
6. El cliente consulta el resumen de partida y el estado o resultado del análisis.

El productor usa una goroutine por partida y espera cada respuesta antes de enviar el siguiente evento de esa partida. Varias partidas avanzan simultáneamente. La API recibe peticiones concurrentes, el estado usa mutex y el envío a Scala utiliza un channel y workers. Las llamadas de red se realizan fuera de los bloqueos de memoria.

## Estructura

| Ruta | Responsabilidad |
|---|---|
| `cmd/server/main.go` | Inicio HTTP, configuración y apagado controlado |
| `cmd/simulator/main.go` | Comando para generar tráfico concurrente |
| `internal/domain/` | Eventos, lotes, estados y resultados de análisis |
| `internal/validation/` | Validación del sobre y datos mínimos por acción |
| `internal/match/` | Estado temporal, duplicados, participantes y ciclo de vida |
| `internal/processing/` | Registro atómico del cierre y lote, cola, workers y reintentos |
| `internal/scala/` | Cliente escrito en Go para llamar a Scala |
| `internal/simulation/` | Escenarios y productor HTTP |
| `internal/transport/http/` | Rutas, códigos HTTP y respuestas JSON |
| `internal/config/` | Lectura y validación de variables de entorno |
| `docs/scala_contract.md` | Contrato que deberá implementar Scala |

## Ejecución local

Requisito: Go 1.22 o posterior. El módulo utiliza la biblioteca estándar y no requiere base de datos.

Desde la raíz del repositorio, primera terminal:

```sh
cd go_service
go run ./cmd/server
```

Go escucha en `http://localhost:8081`. Su URL de análisis por defecto es `http://localhost:8080/analyze`.

En otra terminal, también dentro de `go_service`:

```sh
go run ./cmd/simulator
```

El comando imprime los `matchId` generados. Cada ejecución usa un prefijo nuevo para no colisionar con IDs existentes.

Para IDs conocidos durante una demostración:

```sh
go run ./cmd/simulator -prefix demo_01 -interval 25ms
```

No reutilizar el mismo prefijo contra un proceso que ya recibió esas partidas. Solo cuando Scala implemente el contrato, agregar `-wait` para verificar también la finalización del análisis:

```sh
go run ./cmd/simulator -prefix demo_02 -wait -timeout 2m
```

`-url` cambia el destino Go; `-interval` controla la separación entre envíos de cada partida. Las marcas de tiempo de los eventos son simuladas y no dependen de esa velocidad. El simulador espera y reintenta respuestas 503 hasta 30 intentos por evento; ante otros errores se detiene para esa partida y termina con código distinto de cero. No reenvía automáticamente una petición cuyo resultado de red sea incierto.

## Escenarios disponibles

| Juego | Partida 1 | Partida 2 |
|---|---|---|
| MMA | Golpes próximos, knockdown y detención | Golpes separados, sin knockdown |
| FOOTBALL | Cinco pases completos del mismo equipo y gol | Secuencia interrumpida y pase incompleto |
| MOTORCYCLE_RACING | Accidente y recuperación de posiciones | Parada sin recuperación al primer lugar |
| FREE_FIRE | Daño de A a B y eliminación de B por C | El mismo atacante daña y elimina |

Estos escenarios proveen datos para demostrar las futuras métricas y patrones Scala. Go no determina si el patrón se cumple. Los casos de entrada inválida, duplicados, saturación y fallos de Scala están cubiertos por las pruebas automatizadas.

## API HTTP

| Método | Ruta | Respuesta |
|---|---|---|
| GET | `/health` | 200: salud del proceso Go; no verifica Scala |
| POST | `/events` | 202: evento aceptado |
| POST | `/events/batch` | 202: lote aceptado; detención parcial ante error |
| GET | `/matches/{matchId}` | 200: resumen sin todos los eventos |
| GET | `/matches/{matchId}/analysis` | 200: estado y, si existe, resultado |
| POST | `/matches/{matchId}/analysis/retry` | 202: nuevo ciclo para un análisis `FAILED`; sin body |

Para `/events` y `/events/batch`: `Content-Type: application/json`, cuerpo máximo de 1 MiB y exactamente un valor JSON. Se rechazan campos desconocidos del sobre; las propiedades adicionales de juego van dentro de `data`.

Evento de ejemplo:

```json
{
  "eventId": "demo_start",
  "timestamp": "2026-10-01T12:00:00Z",
  "gameId": "FREE_FIRE",
  "matchId": "demo",
  "playerId": "SYSTEM",
  "type": "MATCH",
  "action": "MATCH_STARTED",
  "data": {}
}
```

Para batch, usar `{"events":[...eventos...]}`. Se decodifica todo el JSON antes de procesar: si el documento no puede decodificarse, se aceptan cero eventos. Si falla la validación o el estado de un evento durante el recorrido, se conserva el prefijo ya aceptado.

La respuesta parcial incluye `accepted`, `failedAtIndex` (índice desde cero), `error` y `readyForAnalysis`. Si una partida cerró antes del error, su análisis queda registrado aunque falle un evento posterior. Al reanudar, enviar desde el evento fallido; no reenviar todo el prefijo aceptado.

El cierre individual devuelve `eventCount`, contando inicio, acciones y fin. `matchStatus=FINISHED` significa partida cerrada; el análisis puede estar todavía pendiente.

### Consultar con PowerShell

Después de ejecutar el simulador con `-prefix demo_01`:

```powershell
Invoke-RestMethod http://localhost:8081/health
Invoke-RestMethod http://localhost:8081/matches/demo_01_FREE_FIRE_1
Invoke-RestMethod http://localhost:8081/matches/demo_01_FREE_FIRE_1/analysis | ConvertTo-Json -Depth 20
```

Tras corregir o iniciar Scala, reintentar una partida que esté en `FAILED`:

```powershell
Invoke-RestMethod -Method Post http://localhost:8081/matches/demo_01_FREE_FIRE_1/analysis/retry
```

### Enviar una partida corta con PowerShell

```powershell
$start = @{
  eventId = 'manual_start'; timestamp = '2026-10-01T12:00:00Z'
  gameId = 'FREE_FIRE'; matchId = 'manual'; playerId = 'SYSTEM'
  type = 'MATCH'; action = 'MATCH_STARTED'; data = @{}
}
$damage = @{
  eventId = 'manual_damage'; timestamp = '2026-10-01T12:00:01Z'
  gameId = 'FREE_FIRE'; matchId = 'manual'; playerId = 'p1'
  type = 'ACTION'; action = 'DAMAGE_DEALT'
  data = @{ targetPlayerId = 'p2'; damage = 50 }
}
$finish = @{
  eventId = 'manual_finish'; timestamp = '2026-10-01T12:00:05Z'
  gameId = 'FREE_FIRE'; matchId = 'manual'; playerId = 'SYSTEM'
  type = 'MATCH'; action = 'MATCH_FINISHED'; data = @{}
}
$body = @{ events = @($start, $damage, $finish) } | ConvertTo-Json -Depth 10
Invoke-RestMethod -Method Post -Uri http://localhost:8081/events/batch -ContentType 'application/json' -Body $body
```

### Estados y errores

| Estado del análisis | Significado |
|---|---|
| `QUEUED` | Lote registrado y esperando worker |
| `PROCESSING` | Petición a Scala en curso |
| `RETRYING` | Espera antes del siguiente intento automático |
| `COMPLETED` | Resultado compatible recibido de Scala |
| `FAILED` | Error permanente o intentos agotados; lote retenido para reintento manual |

La consulta incluye `matchId`, `gameId`, `status`, `eventCount`, `attempts`, `createdAt`, `updatedAt`, `lastError` cuando corresponda y `result` solo al completar. Consultar una partida activa devuelve 409; una inexistente, 404. Un análisis `FAILED` se consulta con HTTP 200 y su estado explícito.

| Código HTTP | Casos |
|---|---|
| 400 | JSON inválido, campos inválidos o falta `matchId` en la consulta |
| 404 | Partida o ruta inexistente |
| 405 | Método incorrecto; incluye `Allow` |
| 409 | Duplicado, conflicto de ciclo de vida/juego/tiempo, análisis no reintentable |
| 413 | Body entrante mayor de 1 MiB |
| 415 | Tipo de contenido distinto de `application/json` |
| 503 | Capacidad agotada o servicio cerrándose; incluye `Retry-After: 1` en errores de ingestión/reintento |

## Garantías y límites del estado

- El `eventId` es único globalmente durante la vida del proceso.
- Las partidas solo abren con `MATCH_STARTED`. Un segundo inicio, juego diferente o eventos posteriores al cierre se rechazan.
- Los participantes incluyen actores, objetivos, víctimas y receptores identificados; `SYSTEM` se excluye. El resumen devuelve sus IDs ordenados.
- Los datos y resultados se entregan mediante copias defensivas.
- La capacidad cubre análisis en cola, en ejecución o esperando reintento. **Cuando se llena, un cierre se rechaza antes de modificar la partida o consumir su `eventId`.** Puede reenviarse exactamente ese cierre después. En ese estado, el control de capacidad puede preceder a otros errores del cierre.
- Los análisis fallidos no ocupan un cupo activo, pero conservan su lote. El reintento manual necesita un cupo.
- No se descartan lotes por fallo de red o saturación mientras el proceso continúa vivo. La idempotencia completa exige cooperación de Scala.
- Todo es **en memoria**: reiniciar elimina partidas, IDs, resultados y lotes pendientes/fallidos. No hay persistencia ni limpieza automática del historial; la capacidad limita trabajo activo, no el tamaño total de la memoria.
- Al recibir SIGINT/SIGTERM, se dejan terminar los handlers y luego se drenan los análisis hasta el plazo configurado. Al vencer, se cancelan las peticiones y se informa la pérdida de estado al salir.

## Configuración

| Variable | Valor por defecto | Uso |
|---|---|---|
| `PORT` | `8081` | Puerto de Go |
| `SCALA_ANALYSIS_URL` | `http://localhost:8080/analyze` | URL completa del endpoint compatible |
| `SCALA_TIMEOUT` | `10s` | Tiempo máximo de cada petición a Scala |
| `ANALYSIS_WORKERS` | `4` | Workers concurrentes |
| `ANALYSIS_CAPACITY` | `128` | Máximo de análisis activos |
| `ANALYSIS_MAX_ATTEMPTS` | `3` | Intentos por ciclo |
| `ANALYSIS_RETRY_BASE` | `1s` | Espera inicial exponencial |
| `ANALYSIS_RETRY_MAX` | `10s` | Tope de espera entre intentos |
| `SHUTDOWN_TIMEOUT` | `30s` | Plazo compartido para apagar HTTP y análisis |

`.env.example` es una referencia; el programa no carga archivos `.env` automáticamente. Se validan puertos, cantidades y duraciones antes de iniciar.

PowerShell:

```powershell
$env:PORT = '8081'
$env:SCALA_ANALYSIS_URL = 'http://localhost:8080/analyze'
go run ./cmd/server
```

Bash:

```sh
PORT=8081 SCALA_ANALYSIS_URL=http://localhost:8080/analyze go run ./cmd/server
```

## Docker del módulo Go

Desde la raíz del repositorio:

```sh
docker build -t gamestats-go ./go_service
```

Cuando exista un servicio Scala compatible y ambos contenedores estén conectados a la misma red Docker:

```sh
docker run --rm --name gamestats-go --network analytics-net -p 8081:8081 -e SCALA_ANALYSIS_URL=http://scala-analytics:8080/analyze gamestats-go
```

La red debe existir y `scala-analytics` debe ser un nombre o alias resoluble en ella. `localhost` dentro del contenedor Go no apunta al contenedor Scala. La configuración Compose conjunta se completará con Scala; no se incluye ni modifica en este trabajo.

La imagen compila `./cmd/server`, ejecuta un binario sin CGO con usuario sin privilegios y contiene certificados para HTTPS. No usa un segundo `main.go` en la raíz de `go_service`.

## Verificación

Desde `go_service`:

```sh
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/server ./cmd/simulator
```

`go test -race` requiere CGO y un compilador C compatible instalado. La prueba HTTP de integración usa un servidor Scala simulado exclusivamente dentro de los tests; prueba ocho partidas concurrentes, lotes íntegros y ordenados, reintentos 503 con clave estable y consulta de resultados. **No demuestra las métricas ni el funcionamiento de Scala real.**

También se comprueban respuestas incompatibles, timeouts, límites, redirecciones, capacidad llena sin consumir el cierre, recuperación manual, snapshots defensivos, batches parciales y apagado con/sin plazo agotado.

Si un entorno de revisión no permite consultar los metadatos Git durante `go build`/`go run`, puede utilizarse `-buildvcs=false`. Esta opción solo desactiva la inclusión de metadatos de versión en el binario.

## Pendientes de integración general

- Adaptar Scala al contrato documentado y verificar las estadísticas y patrones con estos escenarios.
- Probar ambos servicios reales y construir/ejecutar sus contenedores.
- Integrar Compose, CI/CD y deployment del proyecto completo.
- Actualizar el README raíz cuando la arquitectura integrada esté confirmada.
