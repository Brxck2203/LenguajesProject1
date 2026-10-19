package gamestats.analytics

import gamestats.analytics.games.*
import gamestats.model.*

/** Punto de entrada del análisis: una función pura `AnalysisRequest => AnalysisResult`.
  *
  * Compone las estadísticas genéricas (válidas para cualquier juego) con las del analizador registrado para el
  * `gameId`. El resultado es determinista: el mismo lote produce siempre el mismo JSON, lo que permite la
  * idempotencia que exige el contrato con Go.
  */
object AnalysisEngine:

  val analyzers: Map[String, GameAnalyzer] =
    List(MmaAnalyzer, FootballAnalyzer, RacingAnalyzer, FreeFireAnalyzer).map(a => a.gameId -> a).toMap

  def supportedGames: List[String] = analyzers.keys.toList.sorted

  def analyze(request: AnalysisRequest): AnalysisResult =
    val ctx = MatchContext(request)
    val generic = GenericAnalytics.playerStats(ctx)
    val analyzer = analyzers.get(request.gameId)

    val specificPlayers = analyzer.fold(Map.empty[String, ujson.Obj])(_.playerStats(ctx))
    val winner = analyzer.flatMap(_.winner(ctx)).orElse(ctx.declaredWinner)

    // Por jugador: campos del juego en primer nivel y actividad genérica en "activity".
    val playerStats = (generic.keySet ++ specificPlayers.keySet).toList.map { player =>
      val base = specificPlayers.getOrElse(player, ujson.Obj())
      val withActivity = Stats.merge(base, ujson.Obj("activity" -> generic.getOrElse(player, ujson.Obj())))
      // En juegos por equipos el ganador es un equipo: gana todo jugador cuyo "team" coincide.
      val team = base.value.get("team").collect { case ujson.Str(t) => t }
      val won = winner.exists(w => w == player || team.contains(w))
      player -> Stats.merge(withActivity, ujson.Obj("isWinner" -> won))
    }.toMap

    val matchStats = Stats.merge(
      GenericAnalytics.matchStats(ctx),
      Stats.merge(
        analyzer.fold(ujson.Obj())(_.matchStats(ctx)),
        ujson.Obj(
          "winner" -> Stats.optional(winner)(ujson.Str(_)),
          "winnerSource" -> (if ctx.declaredWinner.isDefined then "DECLARED"
                             else if winner.isDefined then "INFERRED"
                             else "NONE"),
          "analyzer" -> analyzer.fold("GENERIC")(_.gameId)
        )
      )
    )

    val patterns = analyzer.fold(List.empty[Pattern])(_.patterns(ctx)).sortBy(p => (p.startSecond, p.name))
    AnalysisResult(request.matchId, request.gameId, playerStats, matchStats, patterns)
