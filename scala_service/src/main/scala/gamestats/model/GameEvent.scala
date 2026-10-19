package gamestats.model

import java.time.{Duration, Instant}

/** Evento inmutable tal como lo entrega Go (sobre común + `data` específico del juego).
  *
  * `data` se conserva como un mapa de valores JSON para que el núcleo de análisis no dependa de un juego en
  * particular: cada analizador lee únicamente los campos que necesita mediante los accesores `str`, `num`, `int` y
  * `bool`, que devuelven `Option` en lugar de lanzar excepciones.
  */
final case class GameEvent(
    eventId: String,
    timestamp: Instant,
    gameId: String,
    matchId: String,
    playerId: String,
    eventType: String,
    action: String,
    data: Map[String, ujson.Value]
):
  def str(field: String): Option[String] =
    data.get(field).collect { case ujson.Str(value) if value.trim.nonEmpty => value }

  def num(field: String): Option[Double] =
    data.get(field).collect { case ujson.Num(value) if !value.isNaN && !value.isInfinite => value }

  def int(field: String): Option[Int] =
    num(field).filter(_.isWhole).map(_.toInt)

  def bool(field: String): Option[Boolean] =
    data.get(field).collect {
      case ujson.True  => true
      case ujson.False => false
    }

  def isLifecycle: Boolean = eventType == EventTypes.Match
  def isSystem: Boolean = playerId == EventTypes.SystemPlayer
  def is(name: String): Boolean = action == name

  /** Actor del evento cuando corresponde a un jugador real (no `SYSTEM`). */
  def actor: Option[String] = Option.when(!isSystem)(playerId)

  /** Jugadores mencionados por el evento: actor, objetivo, víctima o receptor. */
  def involvedPlayers: Set[String] =
    (actor.toList ++ EventTypes.ParticipantFields.flatMap(str))
      .filterNot(_ == EventTypes.SystemPlayer)
      .toSet

/** Lote que Go envía cuando una partida se cierra. Los eventos llegan ordenados por `timestamp`. */
final case class AnalysisRequest(matchId: String, gameId: String, events: List[GameEvent])

/** Vista derivada de una partida que comparten todos los analizadores.
  *
  * Se calcula una sola vez por análisis; todos sus campos son valores inmutables.
  */
final case class MatchContext(request: AnalysisRequest):
  val events: List[GameEvent] = request.events.sortBy(_.timestamp) // sortBy es estable: respeta empates
  val start: Instant =
    events.find(_.is(EventTypes.MatchStarted)).map(_.timestamp).getOrElse(events.head.timestamp)
  val finish: Option[GameEvent] = events.findLast(_.is(EventTypes.MatchFinished))
  val end: Instant = finish.map(_.timestamp).getOrElse(events.last.timestamp)

  /** Eventos de juego (sin inicio ni fin). Incluye acciones globales como `ZONE_SHRINK`. */
  val actions: List[GameEvent] = events.filterNot(_.isLifecycle)

  /** Acciones realizadas por jugadores reales. */
  val playerActions: List[GameEvent] = actions.filterNot(_.isSystem)

  val participants: List[String] = actions.flatMap(_.involvedPlayers).distinct.sorted

  /** Segundos transcurridos desde el inicio de la partida. */
  def second(event: GameEvent): Double = seconds(start, event.timestamp)

  val durationSeconds: Double = seconds(start, end)

  def ofAction(names: String*): List[GameEvent] =
    val wanted = names.toSet
    actions.filter(event => wanted.contains(event.action))

  /** Ganador declarado explícitamente por el productor en `MATCH_FINISHED.data`. */
  val declaredWinner: Option[String] =
    finish.flatMap(event => event.str("winnerPlayerId").orElse(event.str("winnerTeamId")))

  private def seconds(from: Instant, to: Instant): Double =
    Duration.between(from, to).toMillis / 1000.0
