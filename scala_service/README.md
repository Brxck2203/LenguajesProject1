# GameStats — módulo Scala (análisis funcional)

Recibe de Go el lote completo de cada partida cerrada y lo transforma en estadísticas por jugador y por partida, patrones detectados y un historial entre partidas. Todo el análisis son funciones puras sobre colecciones inmutables. Solo el servidor HTTP y la escritura de archivos tienen efectos.

## Estructura

| Paquete | Contenido |
|---|---|
| `gamestats.model` | `GameEvent`, `AnalysisRequest`, `MatchContext` (vista derivada de la partida), `AnalysisResult`, `Pattern`, nombres de juegos y acciones |
| `gamestats.json` | `JsonCodec.decodeRequest: String => Either[String, AnalysisRequest]` y lectura de resultados guardados |
| `gamestats.analytics` | `AnalysisEngine` (composición), `GameAnalyzer` (contrato por juego), `GenericAnalytics`, `CrossMatchStats`, `Stats` (utilidades puras) |
| `gamestats.analytics.games` | `MmaAnalyzer`, `FootballAnalyzer`, `RacingAnalyzer`, `FreeFireAnalyzer` |
| `gamestats.server` | `Api` (función `HttpRequest => HttpResponse`) y `ResultStore` (idempotencia y persistencia) |
| `gamestats.Main` | Borde imperativo: configuración y servidor HTTP con pool de hilos |

## API

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/` | Índice y juegos con analizador propio |
| GET | `/health` | Salud y partidas analizadas |
| POST | `/analyze` | Analiza una partida ([contrato](../docs/contrato-go-scala.md)) |
| GET | `/results` | Partidas analizadas (ganador, patrones, enlace) |
| GET | `/results/{matchId}` | Resultado completo |
| GET | `/stats/games` | Resumen por juego |
| GET | `/stats/games/{gameId}/players` | Historial de cada jugador: partidas, victorias, total y promedio de cada métrica |

Las estadísticas y los patrones de cada juego se describen en [docs/juegos-y-estadisticas.md](../docs/juegos-y-estadisticas.md).

## Agregar un juego

```scala
object ChessAnalyzer extends GameAnalyzer:
  val gameId = "CHESS"
  def playerStats(ctx: MatchContext) =
    ctx.participants.map(p => p -> ujson.Obj("moves" -> ctx.playerActions.count(_.playerId == p))).toMap
  def matchStats(ctx: MatchContext) = ujson.Obj("totalMoves" -> ctx.ofAction("MOVE").size)
  def patterns(ctx: MatchContext) = Nil
  def inferredWinner(ctx: MatchContext) = None
```

Luego se agrega `ChessAnalyzer` a la lista de `AnalysisEngine.analyzers`. Sin ese paso, el juego igual recibe las estadísticas genéricas.

## Configuración

| Variable | Por defecto | Uso |
|---|---|---|
| `PORT` | `8080` | Puerto HTTP |
| `RESULTS_DIR` | (vacío: solo memoria) | Carpeta donde se guarda `<matchId>.json`; se recarga al iniciar (Docker usa `/data/results`) |
| `HTTP_THREADS` | `8` | Hilos que atienden peticiones |

## Comandos

```sh
sbt test        # pruebas (munit)
sbt run         # servidor en :8080
sbt assembly    # target/scala-3.3.1/gamestats-analytics.jar → java -jar …
```

Las pruebas cubren cada analizador con partidas que cumplen y que no cumplen cada patrón, el motor (juego sin analizador, ganador declarado o inferido, determinismo), la decodificación y sus errores, y la API: contrato, idempotencia, conflicto 409, códigos de error, estadísticas entre partidas y recarga desde disco.
