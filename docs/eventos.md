# Especificación del formato de eventos

Este documento es el contrato para cualquier productor de eventos: el simulador actual y la consola de juegos de la segunda etapa del curso. Si un productor respeta este formato, GameStats puede procesar sus partidas sin conocer cómo está implementado.

El catálogo completo también se puede consultar en vivo en `GET http://localhost:8081/catalog`. La fuente de verdad es [`go_service/internal/validation/event_catalog.json`](../go_service/internal/validation/event_catalog.json).

## 1. Sobre común

Todos los eventos, sin importar el juego, tienen exactamente estos ocho campos:

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `eventId` | texto | sí | Identificador único global. Un `eventId` repetido se rechaza (409). |
| `timestamp` | texto RFC 3339 | sí | Instante en que ocurrió el hecho dentro del juego, por ejemplo `2026-10-01T12:00:05Z` o `2026-10-01T06:00:05.250-06:00`. |
| `gameId` | texto | sí | Juego que produjo el evento (`MMA`, `FOOTBALL`, `MOTORCYCLE_RACING`, `FREE_FIRE`, o cualquier otro declarado en el catálogo). |
| `matchId` | texto | sí | Partida a la que pertenece. Todos los eventos de una partida comparten `matchId` y `gameId`. |
| `playerId` | texto | sí | Actor del evento. Los eventos del sistema (inicio, fin, cierre de zona) usan `SYSTEM`. |
| `type` | texto | sí | `MATCH` para el ciclo de vida, `ACTION` para todo lo demás. |
| `action` | texto | sí | Acción concreta (`PASS`, `LAP_COMPLETED`, …). |
| `data` | objeto JSON | sí (puede ser `{}`) | Información específica del juego y de la acción. |

Los campos desconocidos en el sobre se rechazan. Todo dato adicional va **dentro de `data`**: así el sobre nunca cambia y cada juego puede crecer sin romper a los demás.

```json
{
  "eventId": "liga_07_evt_0042",
  "timestamp": "2026-10-01T12:03:18Z",
  "gameId": "FOOTBALL",
  "matchId": "liga_07",
  "playerId": "LEONES_05",
  "type": "ACTION",
  "action": "PASS",
  "data": { "receiverPlayerId": "LEONES_09", "completed": true, "teamId": "LEONES", "distanceMeters": 18 }
}
```

## 2. Relación entre eventos, partidas y jugadores

```
juego (gameId) 1 ── * partida (matchId) 1 ── * evento (eventId) * ── 1..n jugadores
```

- Una **partida** empieza con `MATCH_STARTED` y termina con `MATCH_FINISHED`. Antes del inicio no se acepta ningún evento; después del cierre tampoco.
- Un evento se relaciona con un jugador como **actor** (`playerId`) y, cuando corresponde, con otro jugador mediante `data.targetPlayerId` (objetivo), `data.victimId` (víctima) o `data.receiverPlayerId` (receptor). Con estos campos Go arma la lista de participantes y Scala relaciona eventos entre jugadores.
- En juegos por equipos, `data.teamId` indica el equipo del actor.
- Los jugadores conservan su identificador entre partidas. Así Scala puede calcular el historial de cada jugador (`/stats/games/{gameId}/players`).

## 3. Ciclo de vida (común a todos los juegos)

| Acción | `type` | `playerId` | `data` opcional |
|---|---|---|---|
| `MATCH_STARTED` | `MATCH` | `SYSTEM` | `mode`, `map`, `track`, `totalLaps` |
| `MATCH_FINISHED` | `MATCH` | `SYSTEM` | `winnerPlayerId`, `winnerTeamId`, `reason` |

El ganador en `MATCH_FINISHED` es opcional. Si el productor lo declara, Scala lo usa (`winnerSource: "DECLARED"`); si no, Scala lo infiere de los eventos (`"INFERRED"`).

Reglas de tiempo: ninguna acción puede ser anterior al inicio, y el cierre no puede ser anterior al último evento recibido. Dentro de esos límites se aceptan eventos que lleguen desordenados; antes del análisis Go los ordena por `timestamp` (orden estable en empates).

## 4. Acciones por juego

Tipos de campo: `string` (texto no vacío), `boolean`, `positiveNumber` (> 0), `positiveInteger` (entero > 0), `nonNegativeInteger` (entero ≥ 0). Los campos obligatorios se exigen siempre; los opcionales se validan solo si vienen. `teamId` es opcional en todos los juegos.

### MMA

| Acción | Obligatorio | Opcional | Significado |
|---|---|---|---|
| `STRIKE_ATTEMPT` | `targetPlayerId` | `strikeType` | Golpe lanzado |
| `STRIKE_LANDED` | `targetPlayerId` | `strikeType` | Golpe que conecta. Se envía **además** de su `STRIKE_ATTEMPT` |
| `TAKEDOWN` | `targetPlayerId` | | Derribo |
| `KNOCKDOWN` | `targetPlayerId` | | El rival cae por un golpe |
| `REFEREE_STOPPAGE` | `victimId` | | El árbitro detiene la pelea a favor del actor |

### FOOTBALL

| Acción | Obligatorio | Opcional | Significado |
|---|---|---|---|
| `PASS` | `receiverPlayerId`, `completed` | `distanceMeters` | Pase; `completed=false` si no llega |
| `GOAL` | | `minute` | Gol del actor para su `teamId` |
| `INTERCEPTION` | `targetPlayerId` | | El actor le roba el balón a `targetPlayerId` |
| `FOUL` | `targetPlayerId` | `card` | Falta del actor |

### MOTORCYCLE_RACING

| Acción | Obligatorio | Opcional | Significado |
|---|---|---|---|
| `LAP_COMPLETED` | `lapNumber`, `lapTimeMs`, `position` | `topSpeedKmh` | Vuelta completada (tiempo en **milisegundos**) |
| `CRASH` | | `position` | Caída |
| `PIT_STOP` | | `position` | Entrada a boxes |
| `REJOIN_RACE` | | `position` | Vuelta a la pista tras un incidente |

### FREE_FIRE (battle royale)

| Acción | Obligatorio | Opcional | Significado |
|---|---|---|---|
| `DAMAGE_DEALT` | `targetPlayerId`, `damage` | `weapon` | Daño causado por el actor |
| `PLAYER_ELIMINATED` | `victimId` | `weapon` | El actor elimina a `victimId` |
| `ZONE_SHRINK` | | `zonePhase` | La zona segura se reduce; `playerId=SYSTEM` |

## 5. Canales de entrada

GameStats acepta los mismos eventos por dos vías. Ambas usan la misma validación y el mismo estado de partidas.

**HTTP (REST)**, en el puerto 8081:

- `POST /events`: un evento. Responde 202 si lo acepta.
- `POST /events/batch`: `{"events":[...]}`. Si un evento falla, se conservan los anteriores y la respuesta indica `failedAtIndex`.
- Errores: 400 (evento inválido), 409 (duplicado o conflicto con el ciclo de vida), 413 (cuerpo mayor de 1 MiB), 415 (no es JSON), 503 (capacidad llena; reintentar con `Retry-After`).

**Archivos**, en la carpeta `data/inbox/` (dentro del contenedor `/data/inbox`). Así trabajará la consola de juegos: escribe archivos y GameStats los lee en tiempo casi real.

- Formatos aceptados: `*.json` con un evento, un arreglo de eventos o `{"events":[...]}`; `*.jsonl` o `*.ndjson` con un evento por línea.
- Los archivos se procesan en orden alfabético. Por eso conviene nombrarlos `<matchId>_<parte con ceros>.json`, por ejemplo `liga_07_0003.json`.
- Para no leer un archivo a medio escribir, escriba primero un nombre temporal (que empiece con `.` o termine en `.tmp`) y luego renómbrelo. Además, Go ignora los archivos modificados hace menos de 300 ms.
- Un archivo procesado sin errores se mueve a `processed/`. Si algún evento falla, el archivo se mueve a `rejected/` junto con `<archivo>.error.json`, que indica cuántos eventos se aceptaron y en qué índice falló cada partida.
- Un archivo puede mezclar partidas: Go lo separa por `matchId` y conserva el orden de cada partida.

## 6. Cómo agregar un juego nuevo

1. Copie el catálogo, agregue el juego en `games` con sus acciones y campos, y apunte `EVENT_CATALOG_PATH` a ese archivo, o edite el catálogo embebido. **Go no necesita cambios de código**: valida y agrupa las partidas del juego nuevo de inmediato.
2. Scala ya calcula para cualquier juego las estadísticas genéricas: duración, eventos, acciones por tipo y por jugador, actividad y ganador declarado. Para métricas y patrones propios del juego, se escribe un objeto que implemente `GameAnalyzer` y se registra en `AnalysisEngine.analyzers`. Ninguna otra parte del sistema cambia.

Ejemplo de catálogo con un juego de ajedrez (sin tocar el código):

```json
{
  "version": 1,
  "games": {
    "CHESS": {
      "actions": {
        "MOVE": { "required": { "from": "string", "to": "string" }, "optional": { "capture": "boolean" } }
      }
    }
  }
}
```
