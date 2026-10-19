package gamestats

import gamestats.TestData.*
import gamestats.analytics.AnalysisEngine
import gamestats.analytics.games.*
import gamestats.model.*
import gamestats.model.Actions.*

class MmaSuite extends munit.FunSuite:
  private def fight(gap: Double, knockdown: Boolean) =
    val strikes = (1 to 3).foldLeft(MatchBuilder(Games.Mma).start.at(0.5, StrikeAttempt, "b", "targetPlayerId" -> "a")) {
      (m, i) =>
        m.at(i * gap, StrikeAttempt, "a", "targetPlayerId" -> "b").at(i * gap, StrikeLanded, "a", "targetPlayerId" -> "b")
    }
    val finished =
      if knockdown then
        strikes.at(3 * gap, Knockdown, "a", "targetPlayerId" -> "b").at(3 * gap + 1, RefereeStoppage, "a", "victimId" -> "b")
      else strikes
    finished.finish(60).context

  test("precisión, diferencial y knockdowns por peleador") {
    val stats = MmaAnalyzer.playerStats(fight(1, knockdown = true))
    assertEquals(num(stats("a"), "strikesAttempted"), 3.0)
    assertEquals(num(stats("a"), "strikeAccuracyPct"), 100.0)
    assertEquals(num(stats("b"), "strikeAccuracyPct"), 0.0)
    assertEquals(num(stats("a"), "strikeDifferential"), 3.0)
    assertEquals(num(stats("b"), "knockdownsSuffered"), 1.0)
  }

  test("ráfaga seguida de knockdown y final por detención") {
    val ctx = fight(1, knockdown = true)
    assertEquals(names(MmaAnalyzer.patterns(ctx)), List("FLURRY_KNOCKDOWN", "KNOCKDOWN_FINISH"))
    assertEquals(MmaAnalyzer.winner(ctx), Some("a"))
    assertEquals(str(MmaAnalyzer.matchStats(ctx), "finishType"), "STOPPAGE")
  }

  test("golpes separados no forman ráfaga") {
    val ctx = fight(10, knockdown = false)
    assertEquals(MmaAnalyzer.patterns(ctx), Nil)
    assertEquals(str(MmaAnalyzer.matchStats(ctx), "finishType"), "DECISION")
  }

  test("remontada: el ganador estuvo 5 golpes abajo") {
    val early = (1 to 5).foldLeft(MatchBuilder(Games.Mma).start) { (m, i) =>
      m.at(i * 10, StrikeLanded, "b", "targetPlayerId" -> "a")
    }
    val ctx = (6 to 12).foldLeft(early) { (m, i) => m.at(i * 10, StrikeLanded, "a", "targetPlayerId" -> "b") }
      .finish(200)
      .context
    val patterns = MmaAnalyzer.patterns(ctx)
    assertEquals(names(patterns), List("COMEBACK_VICTORY"))
    assertEquals(patterns.head.details("maxDeficit").num, 5.0)
  }

class FootballSuite extends munit.FunSuite:
  private def passes(interrupted: Boolean) =
    val withPasses = (1 to 5).foldLeft(MatchBuilder(Games.Football).start) { (m, i) =>
      val (from, to) = if i % 2 == 0 then ("p2", "p1") else ("p1", "p2")
      val ok = !interrupted || i != 4
      val m2 = m.at(i * 3, Pass, from, "receiverPlayerId" -> to, "completed" -> ok, "teamId" -> "home")
      if interrupted && i == 3 then m2.at(10, Foul, "p3", "targetPlayerId" -> "p1", "teamId" -> "away") else m2
    }
    withPasses.at(18, Goal, "p2", "teamId" -> "home").finish(60).context

  test("jugada colectiva de 5 pases termina en gol con asistencia") {
    val ctx = passes(interrupted = false)
    assertEquals(names(FootballAnalyzer.patterns(ctx)), List("TEAM_BUILDUP_GOAL"))
    val stats = FootballAnalyzer.playerStats(ctx)
    assertEquals(num(stats("p1"), "assists"), 1.0)
    assertEquals(num(stats("p2"), "goals"), 1.0)
    assertEquals(num(stats("p2"), "goalContributions"), 1.0)
    assertEquals(FootballAnalyzer.winner(ctx), Some("home"))
  }

  test("una falta y un pase fallado cortan la secuencia") {
    val ctx = passes(interrupted = true)
    assertEquals(FootballAnalyzer.patterns(ctx), Nil)
    val stats = FootballAnalyzer.playerStats(ctx)
    assertEquals(num(stats("p2"), "passAccuracyPct"), 50.0)
    assertEquals(num(stats("p1"), "foulsReceived"), 1.0)
    assertEquals(str(stats("p3"), "team"), "away")
  }

  test("posesión y marcador por equipo; remontada") {
    val ctx = MatchBuilder(Games.Football).start
      .at(1, Pass, "a1", "receiverPlayerId" -> "a2", "completed" -> true, "teamId" -> "away")
      .at(2, Goal, "a2", "teamId" -> "away")
      .at(10, Pass, "h1", "receiverPlayerId" -> "h2", "completed" -> true, "teamId" -> "home")
      .at(11, Goal, "h2", "teamId" -> "home")
      .at(20, Goal, "h1", "teamId" -> "home")
      .finish(90)
      .context
    val stats = FootballAnalyzer.matchStats(ctx)
    assertEquals(stats("score")("home").num, 2.0)
    assertEquals(stats("teams")("away")("possessionPct").num, 50.0)
    assertEquals(str(stats, "result"), "home")
    assertEquals(names(FootballAnalyzer.patterns(ctx)), List("COMEBACK_VICTORY"))
  }

class RacingSuite extends munit.FunSuite:
  private def race(incident: String, positions: List[Int]) =
    val firstLap = MatchBuilder(Games.MotorcycleRacing).start
      .at(90, LapCompleted, "r2", "lapNumber" -> 1, "lapTimeMs" -> 90000, "position" -> 2)
      .at(91, LapCompleted, "r1", "lapNumber" -> 1, "lapTimeMs" -> 91000, "position" -> 4)
      .at(100, incident, "r1", "position" -> 8)
      .at(120, RejoinRace, "r1", "position" -> 8)
    positions.zipWithIndex.foldLeft(firstLap) { case (m, (position, i)) =>
      val lap = i + 2
      m.at(100 + lap * 100, LapCompleted, "r1", "lapNumber" -> lap, "lapTimeMs" -> 100000, "position" -> position)
        .at(90 + lap * 100, LapCompleted, "r2", "lapNumber" -> lap, "lapTimeMs" -> 100000, "position" -> (if position == 1 then 2 else 1))
    }.finish(600).context

  test("recuperación después de una caída hasta el primer lugar") {
    val ctx = race(Crash, List(5, 3, 1))
    val patterns = RacingAnalyzer.patterns(ctx).filter(_.name == "COMEBACK_AFTER_INCIDENT")
    assertEquals(patterns.size, 1)
    assertEquals(patterns.head.details("positionsGained").num, 7.0)
    assertEquals(patterns.head.details("reachedLead").bool, true)
    assertEquals(RacingAnalyzer.winner(ctx), Some("r1"))
  }

  test("parada en boxes sin recuperación no es remontada") {
    val ctx = race(PitStop, List(8, 8, 8))
    assertEquals(RacingAnalyzer.patterns(ctx).filter(_.name == "COMEBACK_AFTER_INCIDENT"), Nil)
    assertEquals(RacingAnalyzer.winner(ctx), Some("r2"))
  }

  test("mejor vuelta, consistencia y cambios de líder") {
    val ctx = race(Crash, List(5, 3, 1))
    val r1 = RacingAnalyzer.playerStats(ctx)("r1")
    assertEquals(num(r1, "bestLapMs"), 91000.0)
    assertEquals(num(r1, "positionsGained"), 3.0)
    assertEquals(num(r1, "crashes"), 1.0)
    val stats = RacingAnalyzer.matchStats(ctx)
    assertEquals(stats("fastestLap")("rider").str, "r2")
    assertEquals(num(stats, "leadChanges"), 1.0)
    // r2 no tuvo incidentes, pero su primera vuelta fue más rápida: variación > 0.5 %, sin patrón de ritmo.
    assertEquals(RacingAnalyzer.patterns(ctx).filter(_.name == "CONSISTENT_PACE"), Nil)
  }

  test("ritmo constante con tres vueltas casi iguales") {
    val ctx = MatchBuilder(Games.MotorcycleRacing).start
      .at(90, LapCompleted, "r1", "lapNumber" -> 1, "lapTimeMs" -> 90000, "position" -> 1)
      .at(181, LapCompleted, "r1", "lapNumber" -> 2, "lapTimeMs" -> 90200, "position" -> 1)
      .at(271, LapCompleted, "r1", "lapNumber" -> 3, "lapTimeMs" -> 90100, "position" -> 1)
      .finish(300)
      .context
    assertEquals(names(RacingAnalyzer.patterns(ctx)), List("CONSISTENT_PACE"))
  }

class FreeFireSuite extends munit.FunSuite:
  private def fight(killer: String) =
    MatchBuilder(Games.FreeFire).start
      .at(1, DamageDealt, "p1", "targetPlayerId" -> "p2", "damage" -> 75)
      .at(3, PlayerEliminated, killer, "victimId" -> "p2", "weapon" -> "SNIPER")
      .at(8, DamageDealt, "p3", "targetPlayerId" -> "p1", "damage" -> 25)
      .at(15, ZoneShrink, EventTypes.SystemPlayer, "zonePhase" -> 2)
      .finish(60)
      .context

  test("third party: otro jugador remata a la víctima dañada") {
    val ctx = fight("p3")
    assertEquals(names(FreeFireAnalyzer.patterns(ctx)), List("THIRD_PARTY"))
    assertEquals(num(FreeFireAnalyzer.matchStats(ctx), "maxZonePhase"), 2.0)
  }

  test("el mismo atacante no es third party; K/D y daño") {
    val ctx = fight("p1")
    assertEquals(FreeFireAnalyzer.patterns(ctx), Nil)
    val p1 = FreeFireAnalyzer.playerStats(ctx)("p1")
    assertEquals(num(p1, "kills"), 1.0)
    assertEquals(num(p1, "kdRatio"), 1.0)
    assertEquals(num(p1, "damageDealt"), 75.0)
    assertEquals(num(p1, "damageTaken"), 25.0)
    assertEquals(FreeFireAnalyzer.playerStats(ctx)("p2")("survived").bool, false)
  }

  test("racha de tres eliminaciones en 30 segundos y único sobreviviente") {
    val ctx = (1 to 3).foldLeft(MatchBuilder(Games.FreeFire).start) { (m, i) =>
      m.at(i * 10, PlayerEliminated, "k", "victimId" -> s"v$i")
    }.finish(60).context
    assertEquals(names(FreeFireAnalyzer.patterns(ctx)), List("KILL_STREAK"))
    assertEquals(FreeFireAnalyzer.winner(ctx), Some("k"))
  }

class EngineSuite extends munit.FunSuite:
  test("un juego sin analizador propio recibe estadísticas genéricas") {
    val request = MatchBuilder("CHESS").start
      .at(5, "MOVE", "white", "from" -> "e2", "to" -> "e4")
      .at(9, "MOVE", "black", "from" -> "e7", "to" -> "e5")
      .finish(30, "winnerPlayerId" -> "white")
      .request
    val result = AnalysisEngine.analyze(request)
    assertEquals(str(result.matchStats, "analyzer"), "GENERIC")
    assertEquals(str(result.matchStats, "winner"), "white")
    assertEquals(str(result.matchStats, "winnerSource"), "DECLARED")
    assertEquals(result.playerStats("white")("activity")("actions").num, 1.0)
    assertEquals(result.playerStats("white")("isWinner").bool, true)
    assertEquals(result.patterns, Nil)
  }

  test("el ganador declarado tiene prioridad sobre el inferido") {
    val request = MatchBuilder(Games.FreeFire).start
      .at(1, PlayerEliminated, "a", "victimId" -> "b")
      .finish(10, "winnerPlayerId" -> "c")
      .request
    assertEquals(str(AnalysisEngine.analyze(request).matchStats, "winner"), "c")
  }

  test("en fútbol ganan todos los jugadores del equipo ganador") {
    val request = MatchBuilder(Games.Football).start
      .at(1, Pass, "h1", "receiverPlayerId" -> "h2", "completed" -> true, "teamId" -> "home")
      .at(2, Goal, "h2", "teamId" -> "home")
      .at(3, Foul, "a1", "targetPlayerId" -> "h1", "teamId" -> "away")
      .finish(90)
      .request
    val players = AnalysisEngine.analyze(request).playerStats
    assertEquals(players("h1")("isWinner").bool, true)
    assertEquals(players("a1")("isWinner").bool, false)
  }

  test("el análisis es determinista") {
    val request = MatchBuilder(Games.Mma).start.at(1, StrikeLanded, "a", "targetPlayerId" -> "b").finish(5).request
    assertEquals(ujson.write(AnalysisEngine.analyze(request).toJson), ujson.write(AnalysisEngine.analyze(request).toJson))
  }
