package gamestats.server

import gamestats.analytics.{AnalysisEngine, CrossMatchStats}
import gamestats.json.JsonCodec

import java.net.URLDecoder
import java.nio.charset.StandardCharsets
import java.time.Clock

final case class HttpRequest(method: String, path: String, headers: Map[String, String], body: String):
  def header(name: String): Option[String] = headers.collectFirst {
    case (key, value) if key.equalsIgnoreCase(name) => value
  }

final case class HttpResponse(status: Int, body: ujson.Value, headers: Map[String, String] = Map.empty)

/** Rutas HTTP del módulo Scala como una función `HttpRequest => HttpResponse`.
  *
  * No depende del servidor concreto, por lo que se prueba sin abrir puertos. El único efecto es el registro en
  * [[ResultStore]].
  */
final class Api(store: ResultStore, clock: Clock = Clock.systemUTC()):

  private val routes = List(
    "GET /" -> "Índice de rutas",
    "GET /health" -> "Salud del servicio",
    "POST /analyze" -> "Analiza el lote de una partida cerrada (contrato con Go)",
    "GET /results" -> "Partidas analizadas",
    "GET /results/{matchId}" -> "Resultado completo de una partida",
    "GET /stats/games" -> "Resumen por juego de todas las partidas analizadas",
    "GET /stats/games/{gameId}/players" -> "Historial agregado de cada jugador en un juego"
  )

  def handle(request: HttpRequest): HttpResponse =
    val segments = request.path.split('/').toList.filter(_.nonEmpty).map(decode)
    (request.method, segments) match
      case ("GET", Nil)                                    => index
      case ("GET", List("health"))                         => health
      case ("POST", List("analyze"))                       => analyze(request)
      case ("GET", List("results"))                        => listResults
      case ("GET", List("results", matchId))               => result(matchId)
      case ("GET", List("stats", "games"))                 => gamesSummary
      case ("GET", List("stats", "games", game, "players")) => players(game)
      case (_, path) =>
        allowedMethod(path).fold(error(404, s"ruta no encontrada: ${request.path}"))(methodNotAllowed)

  private def allowedMethod(path: List[String]): Option[String] = path match
    case Nil | List("health") | List("results") | List("results", _) | List("stats", "games") => Some("GET")
    case List("stats", "games", _, "players")                                                 => Some("GET")
    case List("analyze")                                                                      => Some("POST")
    case _                                                                                    => None

  private def index = HttpResponse(
    200,
    ujson.Obj(
      "service" -> "gamestats-scala-analytics",
      "supportedGames" -> ujson.Arr.from(AnalysisEngine.supportedGames),
      "routes" -> ujson.Arr.from(routes.map((route, text) => ujson.Obj("route" -> route, "description" -> text)))
    )
  )

  private def health = HttpResponse(
    200,
    ujson.Obj("status" -> "ok", "service" -> "scala-analytics", "analyzedMatches" -> store.all.size)
  )

  private def analyze(request: HttpRequest): HttpResponse =
    val isJson = request.header("Content-Type").exists(_.toLowerCase.startsWith("application/json"))
    if !isJson then error(415, "Content-Type debe ser application/json")
    else
      JsonCodec.decodeRequest(request.body) match
        case Left(message) => error(400, message)
        case Right(batch) =>
          val hash = ResultStore.sha256(request.body)
          // Si ya existe un resultado para esta partida no se recalcula (idempotencia).
          val outcome = store.get(batch.matchId) match
            case Some(existing) if existing.bodyHash == hash => RegisterOutcome.AlreadyExists(existing)
            case Some(existing)                              => RegisterOutcome.Conflict(existing)
            case None =>
              store.register(StoredResult(hash, clock.instant(), AnalysisEngine.analyze(batch)))
          outcome match
            case RegisterOutcome.Created(stored)       => withKey(HttpResponse(200, stored.result.toJson), request)
            case RegisterOutcome.AlreadyExists(stored) =>
              withKey(HttpResponse(200, stored.result.toJson, Map("Idempotent-Replay" -> "true")), request)
            case RegisterOutcome.Conflict(_) =>
              error(409, s"la partida ${batch.matchId} ya fue analizada con un lote diferente")

  private def withKey(response: HttpResponse, request: HttpRequest): HttpResponse =
    request.header("Idempotency-Key").fold(response)(key =>
      response.copy(headers = response.headers + ("Idempotency-Key" -> key))
    )

  private def listResults = HttpResponse(
    200,
    ujson.Obj(
      "count" -> store.all.size,
      "results" -> ujson.Arr.from(store.all.map { stored =>
        ujson.Obj(
          "matchId" -> stored.result.matchId,
          "gameId" -> stored.result.gameId,
          "analyzedAt" -> stored.analyzedAt.toString,
          "winner" -> stored.result.matchStats.value.getOrElse("winner", ujson.Null),
          "patterns" -> ujson.Arr.from(stored.result.patterns.map(_.name)),
          "url" -> s"/results/${stored.result.matchId}"
        )
      })
    )
  )

  private def result(matchId: String) =
    store.get(matchId).fold(error(404, s"no hay resultado para $matchId")) { stored =>
      HttpResponse(200, ujson.Obj("analyzedAt" -> stored.analyzedAt.toString, "result" -> stored.result.toJson))
    }

  private def gamesSummary = HttpResponse(200, CrossMatchStats.gamesSummary(store.all.map(_.result)))

  private def players(gameId: String) =
    HttpResponse(
      200,
      ujson.Obj("gameId" -> gameId, "players" -> CrossMatchStats.playersSummary(store.all.map(_.result), gameId))
    )

  private def methodNotAllowed(allow: String) =
    error(405, "método no permitido").copy(headers = Map("Allow" -> allow))

  private def error(status: Int, message: String) = HttpResponse(status, ujson.Obj("error" -> message))

  private def decode(segment: String): String = URLDecoder.decode(segment, StandardCharsets.UTF_8)
