# Contrato base de eventos

## Propósito

Este documento define el formato JSON común que utiliza GameStats para transportar eventos desde una fuente externa simulada hacia el módulo de ingestión en Go y, posteriormente, hacia el módulo analítico en Scala.

Go valida el contrato, controla el flujo, agrupa los eventos por partida y prepara el lote para Scala. Scala recibe el lote y calcula las estadísticas y patrones definidos por el equipo.

## Evento base

```json
{
  "eventId": "evt_987654",
  "timestamp": "2026-09-27T10:15:30Z",
  "gameId": "FREE_FIRE",
  "matchId": "match_001",
  "playerId": "player_123",
  "type": "ACTION",
  "action": "PLAYER_ELIMINATED",
  "data": {
    "victimId": "player_456",
    "weapon": "SNIPER",
    "damage": 150
  }
}
```

## Campos

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---:|---|
| `eventId` | string | Sí | Identificador único del evento. |
| `timestamp` | string | Sí | Fecha y hora en formato RFC 3339/ISO 8601 UTC. |
| `gameId` | string | Sí | Juego que produjo el evento, por ejemplo `MMA`, `FOOTBALL`, `MOTORCYCLE_RACING` o `FREE_FIRE`. |
| `matchId` | string | Sí | Identificador de la partida a la que pertenece el evento. |
| `playerId` | string | Sí | Jugador que ejecutó o generó la acción. Para eventos globales se usa `SYSTEM`. |
| `type` | string | Sí | Categoría general del evento. En el contrato actual se usa `ACTION` para eventos de juego y `MATCH` para eventos de ciclo de vida. |
| `action` | string | Sí | Acción específica realizada, por ejemplo `PASS`, `STRIKE_LANDED`, `LAP_COMPLETED` o `PLAYER_ELIMINATED`. |
| `data` | object | Sí | Información particular de la acción; cambia según el juego y la acción. |

## Reglas generales

- Todos los eventos deben tener los ocho campos del evento base.
- `eventId` debe ser único dentro del flujo de eventos recibido.
- `timestamp` debe estar expresado en UTC y ser válido según RFC 3339.
- `gameId`, `matchId`, `playerId`, `type` y `action` no pueden estar vacíos.
- `data` siempre debe ser un objeto JSON, aun cuando no contenga propiedades.
- `playerId` identifica al actor de la acción. En eventos globales de partida como `MATCH_STARTED` y `MATCH_FINISHED`, el valor será `SYSTEM`.
- Los eventos de una misma partida se relacionan mediante `matchId`.
- Go valida el sobre común y los campos mínimos requeridos por cada acción; Scala realiza el análisis funcional, las estadísticas y la detección de patrones.

## Eventos de ciclo de vida

Los siguientes eventos permiten a Go abrir, administrar y cerrar el estado temporal de una partida:

```json
{
  "eventId": "match_001_start",
  "timestamp": "2026-09-27T10:00:00Z",
  "gameId": "FREE_FIRE",
  "matchId": "match_001",
  "playerId": "SYSTEM",
  "type": "MATCH",
  "action": "MATCH_STARTED",
  "data": {}
}
```

```json
{
  "eventId": "match_001_end",
  "timestamp": "2026-09-27T10:25:00Z",
  "gameId": "FREE_FIRE",
  "matchId": "match_001",
  "playerId": "SYSTEM",
  "type": "MATCH",
  "action": "MATCH_FINISHED",
  "data": {}
}
```

## Acciones soportadas y datos mínimos

La validación inicial en Go verifica los datos mínimos de las siguientes acciones. Se pueden ampliar sin cambiar la estructura común del contrato.

| Juego | Acción | Campos mínimos dentro de `data` |
|---|---|---|
| MMA | `STRIKE_ATTEMPT` | `targetPlayerId` |
| MMA | `STRIKE_LANDED` | `targetPlayerId` |
| MMA | `TAKEDOWN` | `targetPlayerId` |
| MMA | `KNOCKDOWN` | `targetPlayerId` |
| MMA | `REFEREE_STOPPAGE` | `victimId` |
| Fútbol | `PASS` | `receiverPlayerId`, `completed` |
| Fútbol | `GOAL` | Ninguno |
| Fútbol | `INTERCEPTION` | `targetPlayerId` |
| Fútbol | `FOUL` | `targetPlayerId` |
| Motos | `LAP_COMPLETED` | `lapNumber`, `lapTimeMs`, `position` |
| Motos | `CRASH` | Ninguno |
| Motos | `PIT_STOP` | Ninguno |
| Motos | `REJOIN_RACE` | Ninguno |
| Battle Royale | `DAMAGE_DEALT` | `targetPlayerId`, `damage` |
| Battle Royale | `PLAYER_ELIMINATED` | `victimId` |
| Battle Royale | `ZONE_SHRINK` | Ninguno |
| Todos | `MATCH_STARTED` | Ninguno |
| Todos | `MATCH_FINISHED` | Ninguno |

## Ejemplo: eliminación en Battle Royale

```json
{
  "eventId": "evt_987654",
  "timestamp": "2026-09-27T10:15:30Z",
  "gameId": "FREE_FIRE",
  "matchId": "match_001",
  "playerId": "player_123",
  "type": "ACTION",
  "action": "PLAYER_ELIMINATED",
  "data": {
    "victimId": "player_456",
    "weapon": "SNIPER",
    "damage": 150
  }
}
```

`playerId` es el jugador que realiza la eliminación y `data.victimId` identifica al jugador eliminado.

## Integración Go–Scala

Cuando una partida recibe `MATCH_FINISHED`, el módulo Go debe agrupar sus eventos por `matchId` y enviar a Scala una solicitud con esta forma:

```json
{
  "matchId": "match_001",
  "gameId": "FREE_FIRE",
  "events": [
    {
      "eventId": "match_001_start",
      "timestamp": "2026-09-27T10:00:00Z",
      "gameId": "FREE_FIRE",
      "matchId": "match_001",
      "playerId": "SYSTEM",
      "type": "MATCH",
      "action": "MATCH_STARTED",
      "data": {}
    }
  ]
}
```

Scala debe devolver resultados estructurados por partida. El contrato de respuesta será definido junto con el módulo de análisis.
