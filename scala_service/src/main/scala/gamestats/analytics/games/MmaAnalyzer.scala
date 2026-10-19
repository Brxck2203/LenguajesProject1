package gamestats.analytics.games

import gamestats.analytics.{GameAnalyzer, Stats}
import gamestats.model.*
import gamestats.model.Actions.*

/** Artes marciales mixtas: cada golpe genera `STRIKE_ATTEMPT` y, si conecta, además `STRIKE_LANDED`. */
object MmaAnalyzer extends GameAnalyzer:
  val gameId: String = Games.Mma

  /** Golpes conectados necesarios antes de un knockdown para considerarlo una ráfaga. */
  val FlurryMinStrikes = 3
  val FlurryWindowSeconds = 5.0
  val FinishWindowSeconds = 10.0
  val ComebackDeficit = 5

  private val target: GameEvent => Option[String] = _.str("targetPlayerId")

  def playerStats(ctx: MatchContext): Map[String, ujson.Obj] =
    val attempts = Stats.countBy(ctx.ofAction(StrikeAttempt))(_.actor)
    val landed = Stats.countBy(ctx.ofAction(StrikeLanded))(_.actor)
    val absorbed = Stats.countBy(ctx.ofAction(StrikeLanded))(target)
    val takedowns = Stats.countBy(ctx.ofAction(Takedown))(_.actor)
    val kdScored = Stats.countBy(ctx.ofAction(Knockdown))(_.actor)
    val kdSuffered = Stats.countBy(ctx.ofAction(Knockdown))(target)

    ctx.participants.map { fighter =>
      val hit = landed.getOrElse(fighter, 0)
      // Un golpe conectado implica un intento, aunque el productor omita STRIKE_ATTEMPT.
      val tried = math.max(attempts.getOrElse(fighter, 0), hit)
      val received = absorbed.getOrElse(fighter, 0)
      fighter -> ujson.Obj(
        "strikesAttempted" -> tried,
        "strikesLanded" -> hit,
        "strikeAccuracyPct" -> Stats.pct(hit, tried),
        "strikesAbsorbed" -> received,
        "strikeDifferential" -> (hit - received),
        "takedowns" -> takedowns.getOrElse(fighter, 0),
        "knockdownsScored" -> kdScored.getOrElse(fighter, 0),
        "knockdownsSuffered" -> kdSuffered.getOrElse(fighter, 0)
      )
    }.toMap

  def matchStats(ctx: MatchContext): ujson.Obj =
    val tried = ctx.ofAction(StrikeAttempt).size
    val hit = ctx.ofAction(StrikeLanded).size
    val stoppage = ctx.ofAction(RefereeStoppage).headOption
    ujson.Obj(
      "totalStrikesAttempted" -> math.max(tried, hit),
      "totalStrikesLanded" -> hit,
      "overallAccuracyPct" -> Stats.pct(hit, math.max(tried, hit)),
      "knockdowns" -> ctx.ofAction(Knockdown).size,
      "takedowns" -> ctx.ofAction(Takedown).size,
      "finishType" -> (if stoppage.isDefined then "STOPPAGE" else "DECISION"),
      "finishSecond" -> Stats.optional(stoppage)(e => ujson.Num(ctx.second(e)))
    )

  def inferredWinner(ctx: MatchContext): Option[String] =
    ctx.ofAction(RefereeStoppage).headOption.flatMap(_.actor).orElse {
      Stats.uniqueMax(Stats.countBy(ctx.ofAction(StrikeLanded))(_.actor).view.mapValues(_.toDouble).toMap)
    }

  def patterns(ctx: MatchContext): List[Pattern] =
    flurryKnockdowns(ctx) ++ knockdownFinishes(ctx) ++ comeback(ctx).toList

  /** Ráfaga: al menos 3 golpes conectados del mismo peleador al mismo rival en 5 s, seguidos de un knockdown. */
  def flurryKnockdowns(ctx: MatchContext): List[Pattern] =
    for
      knockdown <- ctx.ofAction(Knockdown)
      attacker <- knockdown.actor.toList
      victim <- target(knockdown).toList
      strikes = ctx.ofAction(StrikeLanded).filter { strike =>
        val gap = ctx.second(knockdown) - ctx.second(strike)
        strike.actor.contains(attacker) && target(strike).contains(victim) && gap >= 0 && gap <= FlurryWindowSeconds
      }
      if strikes.size >= FlurryMinStrikes
    yield Pattern(
      name = "FLURRY_KNOCKDOWN",
      description = s"$attacker conectó ${strikes.size} golpes en menos de ${FlurryWindowSeconds.toInt} s y derribó a $victim",
      players = List(attacker, victim),
      eventIds = strikes.map(_.eventId) :+ knockdown.eventId,
      startSecond = ctx.second(strikes.head),
      endSecond = ctx.second(knockdown),
      details = ujson.Obj("strikesInFlurry" -> strikes.size)
    )

  /** Final por knockdown: detención arbitral a favor de quien derribó al rival pocos segundos antes. */
  def knockdownFinishes(ctx: MatchContext): List[Pattern] =
    for
      stoppage <- ctx.ofAction(RefereeStoppage)
      winner <- stoppage.actor.toList
      loser <- stoppage.str("victimId").toList
      knockdown <- ctx.ofAction(Knockdown).findLast { kd =>
        val gap = ctx.second(stoppage) - ctx.second(kd)
        kd.actor.contains(winner) && target(kd).contains(loser) && gap >= 0 && gap <= FinishWindowSeconds
      }.toList
    yield Pattern(
      name = "KNOCKDOWN_FINISH",
      description = s"$winner terminó la pelea tras derribar a $loser",
      players = List(winner, loser),
      eventIds = List(knockdown.eventId, stoppage.eventId),
      startSecond = ctx.second(knockdown),
      endSecond = ctx.second(stoppage),
      details = ujson.Obj("secondsFromKnockdown" -> Stats.round2(ctx.second(stoppage) - ctx.second(knockdown)))
    )

  /** Remontada: el ganador llegó a estar 5 o más golpes conectados por debajo de su rival. */
  def comeback(ctx: MatchContext): Option[Pattern] =
    for
      winner <- winner(ctx)
      strikes = ctx.ofAction(StrikeLanded).filter(_.actor.isDefined)
      // Diferencial acumulado del ganador después de cada golpe (scanLeft conserva la historia completa).
      timeline = strikes.scanLeft(0) { (diff, strike) =>
        if strike.actor.contains(winner) then diff + 1 else diff - 1
      }.tail.zip(strikes)
      (worstDiff, worstStrike) <- timeline.minByOption(_._1)
      if worstDiff <= -ComebackDeficit
    yield Pattern(
      name = "COMEBACK_VICTORY",
      description = s"$winner ganó después de estar ${-worstDiff} golpes por debajo",
      players = List(winner),
      eventIds = List(worstStrike.eventId),
      startSecond = ctx.second(worstStrike),
      endSecond = ctx.durationSeconds,
      details = ujson.Obj("maxDeficit" -> -worstDiff)
    )
