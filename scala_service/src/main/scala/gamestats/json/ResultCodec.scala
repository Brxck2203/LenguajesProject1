package gamestats.json

import gamestats.model.*

import scala.util.Try

/** Lectura de resultados ya persistidos en disco (para recuperar el historial al reiniciar). */
object ResultCodec:

  def decodeResult(json: ujson.Value): Either[String, AnalysisResult] =
    Try {
      val obj = json.obj
      val players = obj("playerStats").obj.toList.map((player, stats) => player -> asObj(stats))
      AnalysisResult(
        matchId = obj("matchId").str,
        gameId = obj("gameId").str,
        playerStats = players.toMap,
        matchStats = asObj(obj("matchStats")),
        patterns = obj("patterns").arr.toList.map(decodePattern)
      )
    }.toEither.left.map(error => s"resultado inválido: ${error.getMessage}")

  private def decodePattern(json: ujson.Value): Pattern =
    val obj = json.obj
    Pattern(
      name = obj("name").str,
      description = obj.get("description").map(_.str).getOrElse(""),
      players = obj.get("players").map(_.arr.toList.map(_.str)).getOrElse(Nil),
      eventIds = obj.get("eventIds").map(_.arr.toList.map(_.str)).getOrElse(Nil),
      startSecond = obj.get("startSecond").map(_.num).getOrElse(0.0),
      endSecond = obj.get("endSecond").map(_.num).getOrElse(0.0),
      details = obj.get("details").map(asObj).getOrElse(ujson.Obj())
    )

  private def asObj(value: ujson.Value): ujson.Obj = ujson.Obj.from(value.obj.toList)
