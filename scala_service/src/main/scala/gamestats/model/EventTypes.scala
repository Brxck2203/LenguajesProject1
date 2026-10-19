package gamestats.model

/** Nombres canónicos compartidos con Go (ver docs/eventos.md). */
object EventTypes:
  val Match = "MATCH"
  val Action = "ACTION"
  val SystemPlayer = "SYSTEM"

  val MatchStarted = "MATCH_STARTED"
  val MatchFinished = "MATCH_FINISHED"

  /** Campos de `data` que identifican a otro jugador involucrado en la acción. */
  val ParticipantFields: List[String] = List("targetPlayerId", "victimId", "receiverPlayerId")

object Games:
  val Mma = "MMA"
  val Football = "FOOTBALL"
  val MotorcycleRacing = "MOTORCYCLE_RACING"
  val FreeFire = "FREE_FIRE"

object Actions:
  // MMA
  val StrikeAttempt = "STRIKE_ATTEMPT"
  val StrikeLanded = "STRIKE_LANDED"
  val Takedown = "TAKEDOWN"
  val Knockdown = "KNOCKDOWN"
  val RefereeStoppage = "REFEREE_STOPPAGE"
  // Fútbol
  val Pass = "PASS"
  val Goal = "GOAL"
  val Interception = "INTERCEPTION"
  val Foul = "FOUL"
  // Motociclismo
  val LapCompleted = "LAP_COMPLETED"
  val Crash = "CRASH"
  val PitStop = "PIT_STOP"
  val RejoinRace = "REJOIN_RACE"
  // Free Fire (battle royale)
  val DamageDealt = "DAMAGE_DEALT"
  val PlayerEliminated = "PLAYER_ELIMINATED"
  val ZoneShrink = "ZONE_SHRINK"
