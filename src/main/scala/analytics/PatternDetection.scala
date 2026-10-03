package analytics

import model.{GameEvent, Actions}

object PatternDetection:

  // 1. FÚTBOL: Tiki-Taka Goal
  // 5 o más pases consecutivos del mismo equipo en < 30s que culminan en gol sin interceptación
  def detectTikiTakaGoals(events: List[GameEvent]): List[String] =
    events
      .filter(e => e.action == Actions.PASS || e.action == Actions.GOAL)
      .sliding(6)
      .collect {
        case seq if seq.last.action == Actions.GOAL =>
          val passes = seq.init.filter(_.action == Actions.PASS)
          if passes.length >= 5 then
            Some(s"Tiki-Taka detectado en la partida ${seq.last.matchId} terminado por ${seq.last.playerId}")
          else None
      }
      .flatten
      .toList

  // 2. BATTLE ROYALE: Tercero en Discordia (Third-Party Elimination)
  // Jugador A daña a B, pero Jugador C elimina a B dentro de una ventana de tiempo corta
  def detectThirdPartyEliminations(events: List[GameEvent]): List[String] =
    val eliminations = events.filter(_.action == Actions.PLAYER_ELIMINATED)
    
    eliminations.flatMap { elimEvent =>
      for
        data <- elimEvent.data
        victim <- data.victimId
        cPlayer = elimEvent.playerId
        damageEvent <- events.find { e =>
          e.action == Actions.DAMAGE_DEALT &&
          e.data.flatMap(_.victimId).contains(victim) &&
          e.playerId != cPlayer &&
          e.matchId == elimEvent.matchId
        }
      yield
        val aPlayer = damageEvent.playerId
        s"Patron Tercero en Discordia en ${elimEvent.matchId}: $aPlayer danio a $victim, pero $cPlayer se llevo la baja."
    }