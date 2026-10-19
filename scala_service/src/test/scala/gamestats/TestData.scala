package gamestats

import gamestats.model.*

import java.time.Instant

/** Constructor compacto de partidas para las pruebas. */
final case class MatchBuilder(gameId: String, matchId: String = "m1", events: Vector[GameEvent] = Vector.empty):
  private val base = Instant.parse("2026-10-01T12:00:00Z")

  def at(second: Double, action: String, player: String = EventTypes.SystemPlayer, data: (String, ujson.Value)*)
      : MatchBuilder =
    val eventType =
      if action == EventTypes.MatchStarted || action == EventTypes.MatchFinished then EventTypes.Match
      else EventTypes.Action
    val event = GameEvent(
      eventId = f"$matchId-${events.size}%03d",
      timestamp = base.plusMillis((second * 1000).toLong),
      gameId = gameId,
      matchId = matchId,
      playerId = player,
      eventType = eventType,
      action = action,
      data = data.toMap
    )
    copy(events = events :+ event)

  def start: MatchBuilder = at(0, EventTypes.MatchStarted)
  def finish(second: Double, data: (String, ujson.Value)*): MatchBuilder =
    at(second, EventTypes.MatchFinished, EventTypes.SystemPlayer, data*)

  def request: AnalysisRequest = AnalysisRequest(matchId, gameId, events.sortBy(_.timestamp).toList)
  def context: MatchContext = MatchContext(request)

object TestData:
  def num(obj: ujson.Obj, field: String): Double = obj(field).num
  def str(obj: ujson.Obj, field: String): String = obj(field).str
  def names(patterns: List[Pattern]): List[String] = patterns.map(_.name)

  /** Serializa un lote igual que lo haría Go. */
  def toJson(request: AnalysisRequest): String =
    ujson.write(
      ujson.Obj(
        "matchId" -> request.matchId,
        "gameId" -> request.gameId,
        "events" -> ujson.Arr.from(request.events.map { e =>
          ujson.Obj(
            "eventId" -> e.eventId,
            "timestamp" -> e.timestamp.toString,
            "gameId" -> e.gameId,
            "matchId" -> e.matchId,
            "playerId" -> e.playerId,
            "type" -> e.eventType,
            "action" -> e.action,
            "data" -> ujson.Obj.from(e.data.toList)
          )
        })
      )
    )
