# Juegos, estadísticas y reglas

## 1. Juegos elegidos y por qué

Elegimos cuatro juegos de naturaleza muy distinta, a propósito, para comprobar que el formato de eventos y el núcleo de análisis no dependen de un único juego:

| Juego | Naturaleza | Lo que obliga a resolver |
|---|---|---|
| **MMA** | Combate 1 contra 1, por golpes | Eventos de un actor contra un objetivo; ráfagas en ventanas cortas de tiempo; final anticipado |
| **Fútbol** | Deporte por equipos | Pertenencia a equipos (`teamId`), cadenas de pases, marcador, posesión |
| **Motociclismo** | Carrera por vueltas | Métricas numéricas (tiempos en ms), posiciones que cambian, incidentes que cuestan tiempo |
| **Free Fire** | Battle royale de 8 a 12 jugadores | Muchos jugadores a la vez, eliminación, sobrevivientes, relaciones entre tres jugadores |

Juntos cubren casi todo lo que pedirá la consola de la segunda etapa: juegos individuales y por equipos, de puntaje, de tiempo y de eliminación.

## 2. Por qué estos eventos

La regla de diseño fue **registrar hechos, no resultados**. Por ejemplo, en vez de enviar "precisión = 60 %", el juego envía cada golpe lanzado y cada golpe conectado, y la estadística se deriva de ellos. Así una misma secuencia de eventos alimenta varias métricas y patrones que el juego no tiene por qué conocer.

- **MMA**: `STRIKE_ATTEMPT`/`STRIKE_LANDED` permiten precisión y diferencial. `KNOCKDOWN` y `REFEREE_STOPPAGE` permiten detectar cómo terminó la pelea. `targetPlayerId` relaciona a los dos peleadores.
- **Fútbol**: `PASS` con `receiverPlayerId` y `completed` permite precisión de pases, asistencias y cadenas de juego. `INTERCEPTION` y `FOUL` cortan cadenas. `teamId` da el marcador y la posesión.
- **Motociclismo**: `LAP_COMPLETED` trae número, tiempo y posición; con eso salen la mejor vuelta, la consistencia y la evolución de posiciones. `CRASH`, `PIT_STOP` y `REJOIN_RACE` explican los cambios bruscos de posición.
- **Free Fire**: `DAMAGE_DEALT` con objetivo y daño, y `PLAYER_ELIMINATED` con víctima, permiten K/D, daño causado y recibido, sobrevivientes y patrones entre varios jugadores. `ZONE_SHRINK` marca las fases de la partida.

## 3. Estadísticas implementadas

Todas se calculan en Scala con funciones puras sobre colecciones inmutables (`filter`, `map`, `flatMap`, `groupBy`, `groupMapReduce`, `foldLeft`, `scanLeft`, `collect`, `zip`, `tails`).

### Genéricas (cualquier juego, incluso sin analizador propio)

| Nivel | Estadística |
|---|---|
| Partida | `durationSeconds`, `eventCount`, `actionCount`, `participantCount`, `participants`, `actionsPerMinute`, `actionCounts` (por tipo), `mostActivePlayer`, `winner`, `winnerSource` |
| Jugador (`activity`) | acciones propias, eventos en que participa, acciones por tipo, primer y último segundo con actividad, `isWinner` |

### MMA

| Nivel | Estadística | Cómo se obtiene |
|---|---|---|
| Jugador | `strikesAttempted`, `strikesLanded`, `strikeAccuracyPct` | conteo agrupado por actor |
| Jugador | `strikesAbsorbed`, **`strikeDifferential`** | relaciona golpes propios con los golpes recibidos (agrupados por `targetPlayerId`) |
| Jugador | `takedowns`, `knockdownsScored`, `knockdownsSuffered` | conteos por actor y por objetivo |
| Partida | totales, `overallAccuracyPct`, `finishType` (`STOPPAGE`/`DECISION`), `finishSecond` | |

### Fútbol

| Nivel | Estadística | Cómo se obtiene |
|---|---|---|
| Jugador | `passesAttempted`, `passesCompleted`, `passAccuracyPct`, `passesReceived` | pases agrupados por actor y por receptor |
| Jugador | `goals`, **`assists`**, `goalContributions` | asistencia = último pase antes del gol, completado hacia el goleador, como máximo 10 s antes (combina `PASS` y `GOAL`) |
| Jugador | `interceptions`, `ballsLost`, `foulsCommitted`, `foulsReceived` | `ballsLost` combina intercepciones sufridas y pases fallados |
| Partida | `score`, `result`, `totalGoals`, `assistedGoals`, y por equipo `possessionPct`, `passAccuracyPct`, `fouls` | |

### Motociclismo

| Nivel | Estadística | Cómo se obtiene |
|---|---|---|
| Jugador | `lapsCompleted`, `bestLapMs`, `worstLapMs`, `averageLapMs`, `totalRaceTimeMs` | tiempos de vuelta |
| Jugador | `lapStdDevMs`, `consistencyPct` | desviación estándar y coeficiente de variación |
| Jugador | `startPosition`, `finalPosition`, `positionsGained`, `bestPosition`, `positionHistory` | evolución de posición |
| Jugador | `crashes`, `pitStops` | |
| Partida | `fastestLap`, `averageLapMs`, `totalLaps`, **`leadChanges`**, `lapLeaders` | líder de cada vuelta comparando la posición de todos los pilotos |

### Free Fire

| Nivel | Estadística | Cómo se obtiene |
|---|---|---|
| Jugador | `kills`, `deaths`, **`kdRatio`** | eliminaciones agrupadas por actor y por `victimId` (si no hay muertes, K/D = kills) |
| Jugador | `damageDealt`, `damageTaken`, `hits`, `averageDamagePerHit`, `killsByWeapon` | suma de `damage` por actor y por objetivo |
| Jugador | `survived`, `eliminatedAtSecond` | |
| Partida | `totalKills`, `totalDamage`, `survivors`, `maxZonePhase`, `firstBlood`, `thirdPartyEliminations` | |

Cubren los mínimos del enunciado: estadística por jugador (todas las anteriores), por partida (todas las de partida) y estadísticas que combinan tipos de eventos distintos (diferencial, asistencias, cambios de líder, K/D, `ballsLost`).

### Entre partidas

| Ruta | Contenido |
|---|---|
| `GET /stats/games` | Por juego: partidas analizadas, eventos totales y promedio, duración promedio, conteo de cada patrón y principales ganadores |
| `GET /stats/games/{gameId}/players` | Por jugador: partidas jugadas, victorias, `winRatePct` y, por cada métrica numérica, total y promedio (por ejemplo `kills.total`, `kills.average`) |

El historial por jugador es genérico: un `foldLeft` recorre todos los campos numéricos de `playerStats` sin conocer sus nombres. Funciona igual para un juego nuevo.

## 4. Reglas y patrones (secuencias y combinaciones de eventos)

Cada patrón detectado indica `name`, `description`, `players`, los `eventIds` que lo forman, `startSecond`, `endSecond` y `details`.

| Juego | Patrón | Regla |
|---|---|---|
| MMA | `FLURRY_KNOCKDOWN` | ≥ 3 `STRIKE_LANDED` del mismo peleador al mismo rival en ≤ 5 s, terminando en un `KNOCKDOWN` de ese peleador |
| MMA | `KNOCKDOWN_FINISH` | `REFEREE_STOPPAGE` a favor de quien derribó a esa víctima ≤ 10 s antes |
| MMA | `COMEBACK_VICTORY` | El ganador llegó a estar ≥ 5 golpes conectados por debajo (diferencial acumulado con `scanLeft`) |
| Fútbol | `TEAM_BUILDUP_GOAL` | Gol precedido de ≥ 5 pases completados seguidos del mismo equipo en ≤ 30 s, sin intercepción, falta, pase fallado ni pase rival en medio |
| Fútbol | `COMEBACK_VICTORY` | El equipo ganador estuvo abajo en el marcador en algún momento |
| Motociclismo | `COMEBACK_AFTER_INCIDENT` | Después de un `CRASH` o `PIT_STOP`, el piloto gana ≥ 3 posiciones hasta su última vuelta (`reachedLead` indica si terminó primero) |
| Motociclismo | `CONSISTENT_PACE` | ≥ 3 vueltas con variación ≤ 0,5 % y sin incidentes |
| Free Fire | `THIRD_PARTY` | Un jugador elimina a una víctima que **otro** jugador había dañado en los 10 s anteriores |
| Free Fire | `KILL_STREAK` | El mismo jugador logra ≥ 3 eliminaciones en una ventana de 30 s (ventana deslizante con `tails`) |

Los umbrales son constantes con nombre al inicio de cada analizador (`FlurryWindowSeconds`, `ThirdPartyWindowSeconds`, …), así se pueden ajustar sin tocar la lógica.

## 5. Datos de prueba

El simulador (`go_service/cmd/simulator`) tiene dos fuentes:

- **`fixtures`**: 8 partidas guionadas, dos por juego, con un caso que cumple cada patrón principal y otro que casi lo cumple (por ejemplo, el mismo atacante daña y elimina, que **no** es *third party*). Sirven como casos de prueba conocidos.
- **`random`**: partidas aleatorias realistas y reproducibles (`-seed`). Peleadores con distinta precisión y agresividad, combinaciones de golpes, equipos con distinta calidad de pase, pilotos con distinto ritmo y regularidad, caídas y paradas en boxes, y battle royale con vida, daño y fases de zona. Los jugadores salen de grupos fijos, así acumulan historial entre partidas.

Ejemplo con 100 partidas aleatorias (`-scenario random -matches 25 -seed 11`): unos 930 eventos por partido de fútbol con 3,2 goles en promedio; 56 % de las peleas de MMA terminan por detención; 60 *third parties* y 9 rachas en 25 battle royales; 5 remontadas después de un incidente en 25 carreras.

## 6. Ejemplo de resultado

Resultado (resumido) de la partida guionada `MMA_1`, tal como lo devuelve `POST /analyze` y lo guarda Scala en `data/results/<matchId>.json`:

```json
{
  "matchId": "demo_MMA_1",
  "gameId": "MMA",
  "analyzerVersion": "1.0.0",
  "playerStats": {
    "demo_MMA_1_p1": {
      "strikesAttempted": 3, "strikesLanded": 3, "strikeAccuracyPct": 100,
      "strikesAbsorbed": 0, "strikeDifferential": 3,
      "knockdownsScored": 1, "knockdownsSuffered": 0, "takedowns": 0,
      "activity": { "actions": 8, "involvedInEvents": 9, "firstActionSecond": 1, "lastActionSecond": 4 },
      "isWinner": true
    }
  },
  "matchStats": {
    "durationSeconds": 5, "eventCount": 11, "actionsPerMinute": 108,
    "totalStrikesLanded": 3, "overallAccuracyPct": 75, "knockdowns": 1,
    "finishType": "STOPPAGE", "finishSecond": 4,
    "winner": "demo_MMA_1_p1", "winnerSource": "INFERRED", "analyzer": "MMA"
  },
  "patterns": [
    {
      "name": "FLURRY_KNOCKDOWN",
      "description": "demo_MMA_1_p1 conectó 3 golpes en menos de 5 s y derribó a demo_MMA_1_p2",
      "players": ["demo_MMA_1_p1", "demo_MMA_1_p2"],
      "eventIds": ["demo_MMA_1_evt_003", "demo_MMA_1_evt_005", "demo_MMA_1_evt_007", "demo_MMA_1_evt_008"],
      "startSecond": 1, "endSecond": 3,
      "details": { "strikesInFlurry": 3 }
    },
    { "name": "KNOCKDOWN_FINISH", "startSecond": 3, "endSecond": 4, "details": { "secondsFromKnockdown": 1 } }
  ]
}
```
