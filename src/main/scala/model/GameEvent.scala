package model

import upickle.default.*

case class EventData(
  victimId: Option[String] = None,
  weapon: Option[String] = None,
  damage: Option[Double] = None,
  lapTime: Option[Double] = None,
  success: Option[Boolean] = None,
  teamId: Option[String] = None,
  zonePhase: Option[Int] = None
)

object EventData:
  def fromJson(v: ujson.Value): EventData =
    EventData(
      victimId = v.obj.get("victimId").map(_.str),
      weapon   = v.obj.get("weapon").map(_.str),
      damage   = v.obj.get("damage").map(_.num),
      lapTime  = v.obj.get("lapTime").map(_.num),
      success  = v.obj.get("success").map(_.bool),
      teamId   = v.obj.get("teamId").map(_.str),
      zonePhase = v.obj.get("zonePhase").map(_.num.toInt)
    )

case class GameEvent(
  eventId: String,
  timestamp: String,
  gameId: String,
  matchId: String,
  playerId: String,
  `type`: String,
  action: String,
  data: Option[EventData] = None
)

object GameEvent:
  def fromJson(jsonStr: String): GameEvent =
    val parsed = ujson.read(jsonStr)
    GameEvent(
      eventId   = parsed("eventId").str,
      timestamp = parsed("timestamp").str,
      gameId    = parsed("gameId").str,
      matchId   = parsed("matchId").str,
      playerId  = parsed("playerId").str,
      `type`    = parsed("type").str,
      action    = parsed("action").str,
      data      = parsed.obj.get("data").map(EventData.fromJson)
    )