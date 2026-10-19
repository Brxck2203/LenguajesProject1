package gamestats.analytics.games

import gamestats.analytics.{GameAnalyzer, Stats}
import gamestats.model.*
import gamestats.model.Actions.*

/** Free Fire (battle royale): daño, eliminaciones y cierre de zona. */
object FreeFireAnalyzer extends GameAnalyzer:
  val gameId: String = Games.FreeFire

  val ThirdPartyWindowSeconds = 10.0
  val KillStreakMin = 3
  val KillStreakWindowSeconds = 30.0

  private val target: GameEvent => Option[String] = _.str("targetPlayerId")
  private val victim: GameEvent => Option[String] = _.str("victimId")
  private val damage: GameEvent => Option[Double] = _.num("damage")

  def playerStats(ctx: MatchContext): Map[String, ujson.Obj] =
    val kills = Stats.countBy(ctx.ofAction(PlayerEliminated))(_.actor)
    val deaths = Stats.countBy(ctx.ofAction(PlayerEliminated))(victim)
    val dealt = Stats.sumBy(ctx.ofAction(DamageDealt))(_.actor, damage)
    val taken = Stats.sumBy(ctx.ofAction(DamageDealt))(target, damage)
    val hits = Stats.countBy(ctx.ofAction(DamageDealt))(_.actor)
    val eliminatedAt = ctx.ofAction(PlayerEliminated).flatMap(e => victim(e).map(_ -> ctx.second(e))).toMap
    val weapons = ctx.ofAction(PlayerEliminated).groupBy(_.playerId)

    ctx.participants.map { player =>
      val k = kills.getOrElse(player, 0)
      val d = deaths.getOrElse(player, 0)
      val totalDealt = dealt.getOrElse(player, 0.0)
      val playerHits = hits.getOrElse(player, 0)
      player -> ujson.Obj(
        "kills" -> k,
        "deaths" -> d,
        "kdRatio" -> Stats.ratio(k, d),
        "damageDealt" -> Stats.round2(totalDealt),
        "damageTaken" -> Stats.round2(taken.getOrElse(player, 0.0)),
        "hits" -> playerHits,
        "averageDamagePerHit" -> (if playerHits > 0 then Stats.round2(totalDealt / playerHits) else 0.0),
        "survived" -> !eliminatedAt.contains(player),
        "eliminatedAtSecond" -> Stats.optional(eliminatedAt.get(player))(ujson.Num(_)),
        "killsByWeapon" -> Stats.countsJson(Stats.countBy(weapons.getOrElse(player, Nil))(_.str("weapon")))
      )
    }.toMap

  def survivors(ctx: MatchContext): List[String] =
    val eliminated = ctx.ofAction(PlayerEliminated).flatMap(victim).toSet
    ctx.participants.filterNot(eliminated.contains)

  def matchStats(ctx: MatchContext): ujson.Obj =
    val eliminations = ctx.ofAction(PlayerEliminated)
    val firstBlood = eliminations.headOption
    ujson.Obj(
      "totalKills" -> eliminations.size,
      "totalDamage" -> Stats.round2(ctx.ofAction(DamageDealt).flatMap(damage).sum),
      "survivors" -> ujson.Arr.from(survivors(ctx)),
      "maxZonePhase" -> Stats.optional(ctx.ofAction(ZoneShrink).flatMap(_.int("zonePhase")).maxOption)(ujson.Num(_)),
      "firstBlood" -> Stats.optional(firstBlood)(e =>
        ujson.Obj(
          "killer" -> e.playerId,
          "victim" -> victim(e).getOrElse(""),
          "second" -> ctx.second(e)
        )
      ),
      "thirdPartyEliminations" -> thirdParties(ctx).size
    )

  /** Ganador: único sobreviviente o, si quedan varios, el sobreviviente con más eliminaciones. */
  def inferredWinner(ctx: MatchContext): Option[String] =
    survivors(ctx) match
      case single :: Nil => Some(single)
      case Nil           => None
      case many =>
        val kills = Stats.countBy(ctx.ofAction(PlayerEliminated))(_.actor)
        Stats.uniqueMax(many.map(p => p -> kills.getOrElse(p, 0).toDouble).toMap)

  def patterns(ctx: MatchContext): List[Pattern] =
    thirdParties(ctx) ++ killStreaks(ctx)

  /** Third party: un jugador elimina a una víctima que otro jugador había dañado en los 10 s previos. */
  def thirdParties(ctx: MatchContext): List[Pattern] =
    for
      elimination <- ctx.ofAction(PlayerEliminated)
      killer <- elimination.actor.toList
      dead <- victim(elimination).toList
      damagers = ctx.ofAction(DamageDealt).filter { hit =>
        val gap = ctx.second(elimination) - ctx.second(hit)
        target(hit).contains(dead) && !hit.actor.contains(killer) && hit.actor.isDefined &&
        gap >= 0 && gap <= ThirdPartyWindowSeconds
      }
      if damagers.nonEmpty
    yield Pattern(
      name = "THIRD_PARTY",
      description = s"$killer eliminó a $dead después de que ${damagers.flatMap(_.actor).distinct.mkString(", ")} lo dañara",
      players = (killer :: dead :: damagers.flatMap(_.actor)).distinct,
      eventIds = damagers.map(_.eventId) :+ elimination.eventId,
      startSecond = ctx.second(damagers.head),
      endSecond = ctx.second(elimination),
      details = ujson.Obj(
        "victim" -> dead,
        "damageByOthers" -> Stats.round2(damagers.flatMap(damage).sum)
      )
    )

  /** Racha: el mismo jugador consigue 3+ eliminaciones dentro de una ventana de 30 s. */
  def killStreaks(ctx: MatchContext): List[Pattern] =
    ctx.ofAction(PlayerEliminated)
      .filter(_.actor.isDefined)
      .groupBy(_.playerId)
      .toList
      .sortBy(_._1)
      .flatMap { (player, kills) =>
        // Ventana deslizante: para cada eliminación se buscan las siguientes dentro de 30 s y se toma la más larga.
        val windows = kills.tails.filter(_.nonEmpty).toList.map { rest =>
          rest.takeWhile(k => ctx.second(k) - ctx.second(rest.head) <= KillStreakWindowSeconds)
        }
        windows.maxByOption(_.size).filter(_.size >= KillStreakMin).map { streak =>
          Pattern(
            name = "KILL_STREAK",
            description = s"$player consiguió ${streak.size} eliminaciones en ${KillStreakWindowSeconds.toInt} s",
            players = player :: streak.flatMap(victim),
            eventIds = streak.map(_.eventId),
            startSecond = ctx.second(streak.head),
            endSecond = ctx.second(streak.last),
            details = ujson.Obj("kills" -> streak.size)
          )
        }
      }
