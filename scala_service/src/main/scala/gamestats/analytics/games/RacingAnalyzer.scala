package gamestats.analytics.games

import gamestats.analytics.{GameAnalyzer, Stats}
import gamestats.model.*
import gamestats.model.Actions.*

/** Motociclismo: cada `LAP_COMPLETED` trae número de vuelta, tiempo en milisegundos y posición. */
object RacingAnalyzer extends GameAnalyzer:
  val gameId: String = Games.MotorcycleRacing

  val ComebackMinPositions = 3
  val ConsistentMaxVariationPct = 0.5
  val ConsistentMinLaps = 3

  final case class Lap(rider: String, number: Int, timeMs: Double, position: Int, event: GameEvent)

  /** Vueltas válidas de cada piloto ordenadas por número (si se repite una vuelta se usa la última). */
  def laps(ctx: MatchContext): Map[String, List[Lap]] =
    ctx.ofAction(LapCompleted)
      .flatMap { event =>
        for
          rider <- event.actor
          number <- event.int("lapNumber")
          time <- event.num("lapTimeMs")
          position <- event.int("position")
        yield Lap(rider, number, time, position, event)
      }
      .groupBy(_.rider)
      .view
      .mapValues(_.groupMapReduce(_.number)(identity)((_, latest) => latest).values.toList.sortBy(_.number))
      .toMap

  private def variationPct(times: List[Double]): Option[Double] =
    for
      avg <- Stats.mean(times)
      sd <- Stats.stdDev(times)
      if avg > 0
    yield sd / avg * 100.0

  def playerStats(ctx: MatchContext): Map[String, ujson.Obj] =
    val lapsByRider = laps(ctx)
    val crashes = Stats.countBy(ctx.ofAction(Crash))(_.actor)
    val pits = Stats.countBy(ctx.ofAction(PitStop))(_.actor)
    ctx.participants.map { rider =>
      val own = lapsByRider.getOrElse(rider, Nil)
      val times = own.map(_.timeMs)
      val start = own.headOption.map(_.position)
      val last = own.lastOption.map(_.position)
      rider -> ujson.Obj(
        "lapsCompleted" -> own.size,
        "bestLapMs" -> Stats.optional(times.minOption)(ujson.Num(_)),
        "worstLapMs" -> Stats.optional(times.maxOption)(ujson.Num(_)),
        "averageLapMs" -> Stats.optional(Stats.mean(times))(v => ujson.Num(Stats.round2(v))),
        "lapStdDevMs" -> Stats.optional(Stats.stdDev(times))(v => ujson.Num(Stats.round2(v))),
        "consistencyPct" -> Stats.optional(variationPct(times))(v => ujson.Num(Stats.round2(math.max(0, 100 - v)))),
        "totalRaceTimeMs" -> times.sum,
        "startPosition" -> Stats.optional(start)(ujson.Num(_)),
        "finalPosition" -> Stats.optional(last)(ujson.Num(_)),
        "positionsGained" -> Stats.optional(start.zip(last))((s, l) => ujson.Num(s - l)),
        "bestPosition" -> Stats.optional(own.map(_.position).minOption)(ujson.Num(_)),
        "positionHistory" -> ujson.Arr.from(own.map(lap => ujson.Num(lap.position))),
        "crashes" -> crashes.getOrElse(rider, 0),
        "pitStops" -> pits.getOrElse(rider, 0)
      )
    }.toMap

  /** Líder de cada vuelta: el piloto con mejor posición registrada en ese número de vuelta. */
  def leadersByLap(ctx: MatchContext): List[(Int, String)] =
    laps(ctx).values.flatten.toList
      .groupBy(_.number)
      .toList
      .sortBy(_._1)
      .flatMap { (number, lapsInNumber) =>
        val best = lapsInNumber.map(_.position).min
        lapsInNumber.filter(_.position == best) match
          case single :: Nil => Some(number -> single.rider)
          case _             => None
      }

  def matchStats(ctx: MatchContext): ujson.Obj =
    val allLaps = laps(ctx).values.flatten.toList
    val fastest = allLaps.minByOption(_.timeMs)
    val leaders = leadersByLap(ctx).map(_._2)
    val leadChanges = leaders.zip(leaders.drop(1)).count((a, b) => a != b)
    ujson.Obj(
      "totalLaps" -> Stats.optional(allLaps.map(_.number).maxOption)(ujson.Num(_)),
      "fastestLap" -> Stats.optional(fastest)(lap =>
        ujson.Obj("rider" -> lap.rider, "lapNumber" -> lap.number, "lapTimeMs" -> lap.timeMs)
      ),
      "averageLapMs" -> Stats.optional(Stats.mean(allLaps.map(_.timeMs)))(v => ujson.Num(Stats.round2(v))),
      "leadChanges" -> leadChanges,
      "lapLeaders" -> ujson.Arr.from(leaders),
      "crashes" -> ctx.ofAction(Crash).size,
      "pitStops" -> ctx.ofAction(PitStop).size
    )

  def inferredWinner(ctx: MatchContext): Option[String] =
    val lapsByRider = laps(ctx).filter(_._2.nonEmpty)
    for
      maxLaps <- lapsByRider.values.map(_.size).maxOption
      contenders = lapsByRider.filter(_._2.size == maxLaps)
      winner <- Stats.uniqueMax(contenders.view.mapValues(own => -own.last.position.toDouble).toMap)
    yield winner

  def patterns(ctx: MatchContext): List[Pattern] =
    comebacksAfterIncident(ctx) ++ consistentPace(ctx)

  /** Recuperación: tras una caída o parada en boxes el piloto gana 3+ posiciones hasta el final. */
  def comebacksAfterIncident(ctx: MatchContext): List[Pattern] =
    val lapsByRider = laps(ctx)
    for
      incident <- ctx.ofAction(Crash, PitStop)
      rider <- incident.actor.toList
      lapsAfter = lapsByRider.getOrElse(rider, Nil).filter(lap => ctx.second(lap.event) > ctx.second(incident))
      rejoin = ctx.ofAction(RejoinRace).find(r => r.actor.contains(rider) && ctx.second(r) >= ctx.second(incident))
      positionAfter <- rejoin.flatMap(_.int("position"))
        .orElse(incident.int("position"))
        .orElse(lapsAfter.headOption.map(_.position))
        .toList
      finalLap <- lapsAfter.lastOption.toList
      gained = positionAfter - finalLap.position
      if gained >= ComebackMinPositions
    yield Pattern(
      name = "COMEBACK_AFTER_INCIDENT",
      description = s"$rider recuperó $gained posiciones después de ${if incident.is(Crash) then "una caída" else "entrar a boxes"}",
      players = List(rider),
      eventIds = (incident :: rejoin.toList ++ lapsAfter.map(_.event)).map(_.eventId),
      startSecond = ctx.second(incident),
      endSecond = ctx.second(finalLap.event),
      details = ujson.Obj(
        "incident" -> incident.action,
        "positionAfterIncident" -> positionAfter,
        "finalPosition" -> finalLap.position,
        "positionsGained" -> gained,
        "reachedLead" -> (finalLap.position == 1)
      )
    )

  /** Ritmo constante: 3+ vueltas con variación de tiempos de 0.5 % o menos y sin incidentes. */
  def consistentPace(ctx: MatchContext): List[Pattern] =
    val incidents = ctx.ofAction(Crash, PitStop).flatMap(_.actor).toSet
    laps(ctx).toList.sortBy(_._1).flatMap { (rider, own) =>
      variationPct(own.map(_.timeMs))
        .filter(v => own.size >= ConsistentMinLaps && v <= ConsistentMaxVariationPct && !incidents.contains(rider))
        .map { variation =>
          Pattern(
            name = "CONSISTENT_PACE",
            description = s"$rider mantuvo sus vueltas con ${Stats.round2(variation)} % de variación",
            players = List(rider),
            eventIds = own.map(_.event.eventId),
            startSecond = ctx.second(own.head.event),
            endSecond = ctx.second(own.last.event),
            details = ujson.Obj("laps" -> own.size, "variationPct" -> Stats.round2(variation))
          )
        }
    }
