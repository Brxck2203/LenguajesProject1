import model.*
import analytics.AggregatedMetrics

@main def runAnalytics(): Unit =
  val sampleEvents = List(
    // MMA Events
    GameEvent("e1", "2026-09-27T10:00:00Z", GameTypes.MMA, "m1", "p1", "ACTION", Actions.STRIKE_ATTEMPT),
    GameEvent("e2", "2026-09-27T10:00:01Z", GameTypes.MMA, "m1", "p1", "ACTION", Actions.STRIKE_LANDED),
    GameEvent("e3", "2026-09-27T10:00:02Z", GameTypes.MMA, "m1", "p1", "ACTION", Actions.STRIKE_ATTEMPT),
    
    // Free Fire Events
    GameEvent("e4", "2026-09-27T10:05:00Z", GameTypes.FREE_FIRE, "m2", "p1", "ACTION", Actions.DAMAGE_DEALT, Some(EventData(damage = Some(120.0)))),
    GameEvent("e5", "2026-09-27T10:05:05Z", GameTypes.FREE_FIRE, "m2", "p1", "ACTION", Actions.PLAYER_ELIMINATED, Some(EventData(victimId = Some("p2"))))
  )

  println("=========================================")
  println("APARTADO 1.2: METRICAS AGREGADAS EN SCALA 3")
  println("=========================================")

  val mmaStats = AggregatedMetrics.mmaPrecisionRate(sampleEvents)
  println(s"Efectividad de golpeo MMA (p1): ${mmaStats.getOrElse("p1", 0.0)}%")

  val brStats = AggregatedMetrics.freeFirePlayerStats(sampleEvents)
  val p1Stats = brStats.getOrElse("p1", AggregatedMetrics.PlayerBRStats(0, 0))
  println(s"Free Fire Jugador p1 - K/D: ${p1Stats.kdRatio} | Danio Total: ${p1Stats.totalDamage}")
  println("=========================================")