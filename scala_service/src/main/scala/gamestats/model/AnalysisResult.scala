package gamestats.model

/** Situación detectada al relacionar varios eventos de una partida. */
final case class Pattern(
    name: String,
    description: String,
    players: List[String],
    eventIds: List[String],
    startSecond: Double,
    endSecond: Double,
    details: ujson.Obj = ujson.Obj()
):
  def toJson: ujson.Obj = ujson.Obj(
    "name" -> name,
    "description" -> description,
    "players" -> ujson.Arr.from(players),
    "eventIds" -> ujson.Arr.from(eventIds),
    "startSecond" -> startSecond,
    "endSecond" -> endSecond,
    "details" -> details
  )

/** Resultado completo del análisis de una partida (contrato Go–Scala). */
final case class AnalysisResult(
    matchId: String,
    gameId: String,
    playerStats: Map[String, ujson.Obj],
    matchStats: ujson.Obj,
    patterns: List[Pattern]
):
  def toJson: ujson.Obj = ujson.Obj(
    "matchId" -> matchId,
    "gameId" -> gameId,
    "analyzerVersion" -> AnalysisResult.AnalyzerVersion,
    "playerStats" -> ujson.Obj.from(playerStats.toList.sortBy(_._1)),
    "matchStats" -> matchStats,
    "patterns" -> ujson.Arr.from(patterns.map(_.toJson))
  )

object AnalysisResult:
  val AnalyzerVersion = "1.0.0"
