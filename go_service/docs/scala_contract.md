# Contrato HTTP de análisis Go–Scala

Este contrato está declarado e implementado del lado de Go. **Scala todavía debe adaptarse**. El servicio Go no usa el endpoint de métricas de ejemplo ni transforma nombres de campos para aparentar compatibilidad.

## Transporte y operación

| Elemento | Definición |
|---|---|
| Método y ruta propuesta en Scala | `POST /analyze` |
| Configuración de Go | `SCALA_ANALYSIS_URL`, URL completa con ruta |
| URL local por defecto | `http://localhost:8080/analyze` |
| URL entre contenedores, cuando exista el servicio con ese nombre | `http://scala-analytics:8080/analyze` |
| Solicitud y respuesta | `Content-Type: application/json` |
| Respuesta satisfactoria | HTTP **200** con resultado completo |
| Límite de respuesta aceptada por Go | 4 MiB |
| Identidad de reintento | Cabecera `Idempotency-Key`, SHA-256 hexadecimal del cuerpo JSON enviado |

El límite de 1 MiB de la API de ingestión Go se aplica **por solicitud entrante**, no al total de una partida. Scala deberá aceptar el lote acumulado o acordar un límite de partida con Go antes de imponer un máximo incompatible.

`202` de Scala no significa análisis terminado y se considera incompatible con este contrato síncrono. No se sigue ninguna redirección HTTP. Cambiar la ruta configurada no adapta el formato ni vuelve equivalentes `/events`, `/analytics` y `/analyze`.

## Solicitud

Tipo Go: `domain.AnalysisRequest`, en `internal/domain/event.go`.

```json
{
  "matchId": "ff_001",
  "gameId": "FREE_FIRE",
  "events": [
    {
      "eventId": "ff_001_start",
      "timestamp": "2026-10-01T12:00:00Z",
      "gameId": "FREE_FIRE",
      "matchId": "ff_001",
      "playerId": "SYSTEM",
      "type": "MATCH",
      "action": "MATCH_STARTED",
      "data": {}
    },
    {
      "eventId": "ff_001_damage",
      "timestamp": "2026-10-01T12:00:01Z",
      "gameId": "FREE_FIRE",
      "matchId": "ff_001",
      "playerId": "p1",
      "type": "ACTION",
      "action": "DAMAGE_DEALT",
      "data": {"targetPlayerId": "p2", "damage": 50}
    },
    {
      "eventId": "ff_001_end",
      "timestamp": "2026-10-01T12:00:05Z",
      "gameId": "FREE_FIRE",
      "matchId": "ff_001",
      "playerId": "SYSTEM",
      "type": "MATCH",
      "action": "MATCH_FINISHED",
      "data": {}
    }
  ]
}
```

Reglas:

1. El lote contiene todos los eventos aceptados de **una sola partida**, incluidos inicio y fin. Todos comparten `matchId` y `gameId` con el sobre exterior.
2. Go conserva internamente el orden de llegada. Para Scala usa orden ascendente por instante (`timestamp`), estable en empates. Scala debe preservar ese orden en empates; no se agrega `sequenceNumber`.
3. El JSON de cada evento conserva sus ocho campos originales. `data` conserva sus propiedades; no se traducen silenciosamente nombres ni unidades.
4. El timestamp es RFC 3339. Go compara instantes, no cadenas; admite offsets y fracciones de segundo. Los ejemplos y el simulador usan UTC.
5. Una acción no puede tener timestamp anterior al inicio. El cierre no puede preceder al último instante ya aceptado. Se permiten acciones fuera de orden de llegada dentro de esos límites.
6. Scala debe ignorar eventos de ciclo de vida para métricas que solo correspondan a jugadores y acciones, sin perderlos para duración u otros análisis de partida.
7. No existe obligación de un modelo de métricas particular en Go: su función es transportar el lote y custodiar el resultado.

## Campos que Scala debe alinear

| Concepto | Nombre canónico | Tipo / interpretación |
|---|---|---|
| Juego de motos | `MOTORCYCLE_RACING` | Valor de `gameId`; no `MOTOGP` |
| Pase completado | `data.completed` | Booleano; no `success` |
| Receptor del pase | `data.receiverPlayerId` | String no vacío |
| Objetivo del daño o golpe | `data.targetPlayerId` | String no vacío |
| Víctima de eliminación o detención | `data.victimId` | String no vacío |
| Tiempo de vuelta | `data.lapTimeMs` | Número positivo en **milisegundos**; no `lapTime` |
| Número de vuelta | `data.lapNumber` | Entero positivo |
| Posición en vuelta completada | `data.position` | Entero positivo |
| Equipo | `data.teamId` | String no vacío cuando esté presente; nunca en la raíz del evento |

`teamId` es opcional en la validación básica para conservar compatibilidad con los eventos ya documentados. El simulador lo incluye en pases, goles y faltas. Un análisis que requiera comprobar el mismo equipo debe exigir ese dato para los eventos que relaciona; no puede asumir un equipo cuando falta.

Para Third-Party, Scala debe relacionar `DAMAGE_DEALT.data.targetPlayerId` con `PLAYER_ELIMINATED.data.victimId`, en la misma partida, respetando precedencia y una ventana temporal que se defina al implementar el patrón. No se debe cambiar `victimId` en las eliminaciones.

## Catálogo validado por Go

| Juego | Acción | `data` mínimo |
|---|---|---|
| Todos | `MATCH_STARTED`, `MATCH_FINISHED` | `{}`; `type=MATCH`, `playerId=SYSTEM` |
| MMA | `STRIKE_ATTEMPT`, `STRIKE_LANDED`, `TAKEDOWN`, `KNOCKDOWN` | `targetPlayerId` |
| MMA | `REFEREE_STOPPAGE` | `victimId` |
| FOOTBALL | `PASS` | `receiverPlayerId`, `completed` |
| FOOTBALL | `GOAL` | `{}`; `teamId` recomendado para patrones por equipo |
| FOOTBALL | `INTERCEPTION`, `FOUL` | `targetPlayerId` |
| MOTORCYCLE_RACING | `LAP_COMPLETED` | `lapNumber`, `lapTimeMs`, `position` |
| MOTORCYCLE_RACING | `CRASH`, `PIT_STOP`, `REJOIN_RACE` | `{}` |
| FREE_FIRE | `DAMAGE_DEALT` | `targetPlayerId`, `damage` positivo |
| FREE_FIRE | `PLAYER_ELIMINATED` | `victimId` |
| FREE_FIRE | `ZONE_SHRINK` | `{}` |

Todas las acciones de juego usan `type=ACTION`. Acciones globales como `ZONE_SHRINK` pueden usar `playerId=SYSTEM`. Las propiedades adicionales van en `data`. Go valida los mínimos de la tabla; no interpreta estadísticamente campos opcionales como `weapon`, `zonePhase` o `position` en un accidente.

El catálogo no incorpora aún `STAND_UP`, `BLOCK` ni `FINISH_LINE_CROSSED`; no deben enviarse como si estuvieran implementadas. Su incorporación requiere ampliar modelos y validación antes de utilizarlas.

## Respuesta de Scala

Tipo Go: `domain.AnalysisResult`, en `internal/domain/analysis.go`.

```json
{
  "matchId": "ff_001",
  "gameId": "FREE_FIRE",
  "playerStats": {
    "p1": {"totalDamage": 50}
  },
  "matchStats": {
    "durationSeconds": 5
  },
  "patterns": []
}
```

El ejemplo ilustra el formato; las métricas son responsabilidad de Scala. Go exige:

- `matchId` y `gameId` iguales a los del lote enviado.
- `playerStats`: objeto JSON, destinado a estadísticas por `playerId`.
- `matchStats`: objeto JSON con estadísticas agregadas de la partida.
- `patterns`: arreglo JSON con los patrones detectados. La estructura de sus elementos se detallará al implementar Scala.
- Las tres secciones deben estar presentes; `{}`, `{}` y `[]` son válidos, pero no `null`.

Go conserva estas secciones como `json.RawMessage`: no fabrica métricas, no impone nombres de estadísticas ni valida la corrección matemática del análisis. Los campos externos desconocidos de respuesta se ignoran. La recepción de un resultado con identidad o estructura incorrecta termina en `FAILED`, sin publicarlo como completado.

## Reintentos e idempotencia

- Se reintentan fallos de conexión/lectura, timeout y HTTP 408, 429 o 5xx.
- No se reintentan automáticamente errores de contrato, HTTP 4xx restantes, redirecciones ni respuestas 2xx distintas de 200.
- Por defecto: hasta tres intentos por ciclo, pausas de 1 s y 2 s; la espera exponencial tiene límite configurable.
- Agotados los intentos, el lote queda en memoria como `FAILED`. Se puede solicitar un nuevo ciclo por `POST /matches/{matchId}/analysis/retry` en Go. `attempts` acumula los intentos de todos los ciclos.
- Todos los intentos del mismo lote usan el mismo cuerpo y `Idempotency-Key`.
- **Scala debe implementar la idempotencia**: conservar o reutilizar el resultado ante repetición de la misma clave y lote; no volver a acumular sus eventos. Debe rechazar contenido contradictorio para la misma identidad de partida. Si dos solicitudes equivalentes llegan simultáneamente, debe coordinarlas.
- La garantía de transporte es de intentos repetibles, no de “exactamente una vez”: un timeout puede ocurrir después de que Scala ya procesó el lote.

## Trabajo pendiente exclusivamente del lado analítico

1. Recibir y validar este lote, con modelos compatibles con los campos canónicos.
2. Analizar cada partida de forma aislada y admitir peticiones concurrentes sin compartir colecciones mutables de manera insegura.
3. Calcular estadísticas por jugador y partida, relaciones entre tipos de eventos y patrones con sus condiciones temporales completas.
4. Devolver el resultado estructurado, no estadísticas internas de muestra.
5. Implementar la idempotencia y asegurar que host, puerto y ruta coincidan con la configuración.

Los archivos Scala existentes no se han modificado como parte de la implementación de Go.
