package gamestats.analytics

import gamestats.model.*

/** Contrato que implementa cada juego.
  *
  * Un analizador es un conjunto de funciones puras `MatchContext => resultado`. Agregar un juego nuevo consiste en
  * escribir otro objeto que implemente este trait y registrarlo en [[AnalysisEngine.analyzers]]; los juegos sin
  * analizador propio reciben igualmente las estadísticas genéricas.
  */
trait GameAnalyzer:
  def gameId: String

  /** Estadísticas específicas por jugador (o piloto/peleador). */
  def playerStats(ctx: MatchContext): Map[String, ujson.Obj]

  /** Estadísticas específicas de la partida. */
  def matchStats(ctx: MatchContext): ujson.Obj

  /** Reglas que relacionan secuencias o combinaciones de eventos. */
  def patterns(ctx: MatchContext): List[Pattern]

  /** Ganador inferido a partir de los eventos cuando el productor no lo declara. */
  def inferredWinner(ctx: MatchContext): Option[String]

  /** Ganador final: el declarado en `MATCH_FINISHED` tiene prioridad sobre el inferido. */
  final def winner(ctx: MatchContext): Option[String] = ctx.declaredWinner.orElse(inferredWinner(ctx))

/** Estadísticas válidas para cualquier juego, aunque no tenga analizador propio. */
object GenericAnalytics:

  def playerStats(ctx: MatchContext): Map[String, ujson.Obj] =
    val byActor = ctx.playerActions.groupBy(_.playerId)
    ctx.participants.map { player =>
      val own = byActor.getOrElse(player, Nil)
      val involved = ctx.actions.count(_.involvedPlayers.contains(player))
      player -> ujson.Obj(
        "actions" -> own.size,
        "involvedInEvents" -> involved,
        "actionsByType" -> Stats.countsJson(Stats.countBy(own)(e => Some(e.action))),
        "firstActionSecond" -> Stats.optional(own.headOption)(e => ujson.Num(ctx.second(e))),
        "lastActionSecond" -> Stats.optional(own.lastOption)(e => ujson.Num(ctx.second(e)))
      )
    }.toMap

  def matchStats(ctx: MatchContext): ujson.Obj =
    val minutes = ctx.durationSeconds / 60.0
    ujson.Obj(
      "durationSeconds" -> ctx.durationSeconds,
      "eventCount" -> ctx.events.size,
      "actionCount" -> ctx.actions.size,
      "participantCount" -> ctx.participants.size,
      "participants" -> ujson.Arr.from(ctx.participants),
      "actionsPerMinute" -> (if minutes > 0 then Stats.round2(ctx.actions.size / minutes) else 0.0),
      "actionCounts" -> Stats.countsJson(Stats.countBy(ctx.actions)(e => Some(e.action))),
      "mostActivePlayer" -> Stats.optional(
        Stats.uniqueMax(Stats.countBy(ctx.playerActions)(_.actor).view.mapValues(_.toDouble).toMap)
      )(ujson.Str(_))
    )
