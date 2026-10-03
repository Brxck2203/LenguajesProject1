package model

object GameTypes:
  inline val MMA        = "MMA"
  inline val FOOTBALL   = "FOOTBALL"
  inline val MOTOGP     = "MOTOGP"
  inline val FREE_FIRE  = "FREE_FIRE"

object Actions:
  inline val STRIKE_ATTEMPT    = "STRIKE_ATTEMPT"
  inline val STRIKE_LANDED     = "STRIKE_LANDED"
  inline val PASS              = "PASS"
  inline val GOAL              = "GOAL"
  inline val LAP_COMPLETED     = "LAP_COMPLETED"
  inline val PLAYER_ELIMINATED = "PLAYER_ELIMINATED"
  inline val DAMAGE_DEALT      = "DAMAGE_DEALT"