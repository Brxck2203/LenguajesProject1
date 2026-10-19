# Contrato HTTP Go → Scala

| Elemento | Valor |
|---|---|
| Método y ruta | `POST /analyze` en Scala (puerto 8080) |
| Configuración en Go | `SCALA_ANALYSIS_URL` (URL completa). En Compose: `http://scala-analytics:8080/analyze` |
| Formato | `Content-Type: application/json` en la solicitud y en la respuesta |
| Respuesta correcta | HTTP **200** con el resultado completo (análisis síncrono) |
| Identidad del lote | Cabecera `Idempotency-Key`: SHA-256 en hexadecimal del cuerpo enviado |
| Límites | Go acepta respuestas de hasta 4 MiB; Scala acepta lotes de hasta 16 MiB |

## Solicitud

Go la envía cuando recibe `MATCH_FINISHED`. Contiene **todos** los eventos aceptados de **una** partida, incluidos el inicio y el fin, ordenados por `timestamp` (orden estable en empates). Cada evento conserva sus ocho campos originales, sin renombrar ni convertir unidades.

```json
{
  "matchId": "ff_001",
  "gameId": "FREE_FIRE",
  "events": [
    { "eventId": "ff_001_start", "timestamp": "2026-10-01T12:00:00Z", "gameId": "FREE_FIRE", "matchId": "ff_001",
      "playerId": "SYSTEM", "type": "MATCH", "action": "MATCH_STARTED", "data": {} },
    { "eventId": "ff_001_dmg", "timestamp": "2026-10-01T12:00:01Z", "gameId": "FREE_FIRE", "matchId": "ff_001",
      "playerId": "p1", "type": "ACTION", "action": "DAMAGE_DEALT", "data": { "targetPlayerId": "p2", "damage": 50 } },
    { "eventId": "ff_001_end", "timestamp": "2026-10-01T12:00:05Z", "gameId": "FREE_FIRE", "matchId": "ff_001",
      "playerId": "SYSTEM", "type": "MATCH", "action": "MATCH_FINISHED", "data": { "winnerPlayerId": "p1" } }
  ]
}
```

Scala vuelve a validar el lote: todos los eventos deben pertenecer a `matchId`/`gameId`, los `eventId` no pueden repetirse, y deben estar `MATCH_STARTED` y `MATCH_FINISHED`. Si algo falla responde **400** con `{"error": "..."}`.

## Respuesta

```json
{
  "matchId": "ff_001",
  "gameId": "FREE_FIRE",
  "analyzerVersion": "1.0.0",
  "playerStats": { "p1": { "kills": 0, "damageDealt": 50, "activity": { "actions": 1 }, "isWinner": true } },
  "matchStats": { "durationSeconds": 5, "eventCount": 3, "winner": "p1", "winnerSource": "DECLARED", "analyzer": "FREE_FIRE" },
  "patterns": []
}
```

- `matchId` y `gameId` deben coincidir con el lote; si no, Go marca el análisis como `FAILED`.
- `playerStats` (objeto por `playerId`), `matchStats` (objeto) y `patterns` (arreglo) son obligatorios. Pueden estar vacíos, pero nunca ser `null`.
- Go guarda las tres secciones tal cual (`json.RawMessage`), sin interpretarlas. Los nombres de las métricas los define Scala ([juegos-y-estadisticas.md](juegos-y-estadisticas.md)). Los campos adicionales, como `analyzerVersion`, se ignoran.

## Códigos de Scala

| Código | Caso | Qué hace Go |
|---|---|---|
| 200 | Resultado nuevo, o repetido para el mismo lote (`Idempotent-Replay: true`) | `COMPLETED` |
| 400 | Lote inválido | `FAILED`, sin reintento automático |
| 409 | El `matchId` ya se analizó con **otro** contenido | `FAILED`, sin reintento automático |
| 415 | `Content-Type` distinto de JSON | `FAILED` |
| 5xx, timeout, error de red | Fallo transitorio | Reintenta con espera exponencial (3 intentos por defecto) |

## Idempotencia

El análisis es una función pura: el mismo lote produce siempre el mismo resultado. Scala guarda el SHA-256 del cuerpo junto al resultado de cada partida (también en disco). Por eso:

- Si Go reintenta tras un timeout en que Scala sí alcanzó a analizar, recibe el mismo resultado sin recalcular.
- Si llegan a la vez dos solicitudes iguales, el registro atómico deja solo una y ambas reciben el mismo resultado.
- Reutilizar un `matchId` con eventos distintos se detecta como conflicto (409) en vez de sobrescribir un resultado guardado.

## Estados del análisis en Go

`GET /matches/{matchId}/analysis` devuelve `status` (`QUEUED`, `PROCESSING`, `RETRYING`, `COMPLETED`, `FAILED`), `attempts`, `lastError` y, cuando termina, `result`. Un análisis `FAILED` se puede volver a encolar con `POST /matches/{matchId}/analysis/retry`.
