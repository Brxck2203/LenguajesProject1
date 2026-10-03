package analytics

import model.{GameEvent, Actions}

object AggregatedMetrics:

  // 1. MMA: Precision Rate (Efectividad de Golpeo) por Jugador
  def mmaPrecisionRate(events: List[GameEvent]): Map[String, Double] =
    events
      .filter(e => e.action == Actions.STRIKE_ATTEMPT || e.action == Actions.STRIKE_LANDED)
      .groupBy(_.playerId)
      .map { case (playerId, playerEvents) =>
        val attempts = playerEvents.count(_.action == Actions.STRIKE_ATTEMPT)
        val landed   = playerEvents.count(_.action == Actions.STRIKE_LANDED)
        val rate     = if attempts > 0 then (landed.toDouble / attempts) * 100.0 else 0.0
        playerId -> rate
      }

  // 2. FÚTBOL: Pass Completion Rate (Efectividad de Pases) por Jugador
  def footballPassCompletionRate(events: List[GameEvent]): Map[String, Double] =
    events
      .filter(_.action == Actions.PASS)
      .groupBy(_.playerId)
      .map { case (playerId, passEvents) =>
        val totalPasses      = passEvents.length
        val successfulPasses = passEvents.flatMap(_.data).count(_.success.contains(true))
        val rate             = if totalPasses > 0 then (successfulPasses.toDouble / totalPasses) * 100.0 else 0.0
        playerId -> rate
      }

  // 3. CARRERAS: Tiempo Promedio por Vuelta por Piloto
  def motoGPLapTimeStats(events: List[GameEvent]): Map[String, (Double, Double)] =
    events
      .filter(_.action == Actions.LAP_COMPLETED)
      .groupBy(_.playerId)
      .map { case (playerId, lapEvents) =>
        val times = lapEvents.flatMap(_.data).flatMap(_.lapTime)
        if times.isEmpty then
          playerId -> (0.0, 0.0)
        else
          val avg = times.sum / times.length
          val min = times.min
          val max = times.max
          val variance = max - min
          playerId -> (avg, variance)
      }

  // 4. BATTLE ROYALE: K/D Ratio y Daño Promedio por Jugador
  case class PlayerBRStats(kdRatio: Double, totalDamage: Double)

  def freeFirePlayerStats(events: List[GameEvent]): Map[String, PlayerBRStats] =
    val kills = events
      .filter(_.action == Actions.PLAYER_ELIMINATED)
      .groupBy(_.playerId)
      .map((pid, evs) => pid -> evs.length)

    val deaths = events
      .filter(_.action == Actions.PLAYER_ELIMINATED)
      .flatMap(_.data)
      .flatMap(_.victimId)
      .groupBy(identity)
      .map((pid, evs) => pid -> evs.length)

    val damageMap = events
      .filter(_.action == Actions.DAMAGE_DEALT)
      .groupBy(_.playerId)
      .map { (pid, evs) =>
        val totalDmg = evs.flatMap(_.data).flatMap(_.damage).foldLeft(0.0)(_ + _)
        pid -> totalDmg
      }

    val allPlayers = (kills.keySet ++ deaths.keySet ++ damageMap.keySet)

    allPlayers.map { pid =>
      val k = kills.getOrElse(pid, 0).toDouble
      val d = deaths.getOrElse(pid, 0).toDouble
      val kd = if d == 0 then k else k / d
      val dmg = damageMap.getOrElse(pid, 0.0)
      pid -> PlayerBRStats(kd, dmg)
    }.toMap