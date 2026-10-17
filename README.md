# GameStats — Go y Scala

Go recibe y valida eventos de MMA, fútbol, motociclismo y Free Fire, conserva partidas en memoria y envía cada partida cerrada a Scala. Scala debe calcular las estadísticas y detectar patrones.

**Estado actual:** el flujo Go, el simulador y su configuración Docker están integrados. Scala conserva su implementación actual: `/analytics` devuelve un ejemplo fijo; todavía falta implementar `POST /analyze` con el [contrato de integración](go_service/docs/scala_contract.md). Por ello se pueden generar y consultar partidas reales en Go, pero sus análisis quedan `FAILED` hasta adaptar Scala. `health: ok` solo confirma que el servidor responde.

## Probar desde Docker Desktop

Abre Docker Desktop y una consola en la raíz del repositorio. No necesitas instalar Go para estos pasos.

```sh
git switch main
git pull --ff-only
docker compose up -d --build
docker compose ps
```

Espera a que Go aparezca `healthy`. La primera compilación de Scala puede tardar. Si un servicio no arranca:

```sh
docker compose logs --tail=100 go-service scala-analytics
```

1. Abre <http://localhost:8081/> para ver las rutas disponibles, y <http://localhost:8081/health> para comprobar Go.
2. Genera ocho partidas (dos por juego, 73 eventos) desde un contenedor temporal:

   ```sh
   docker compose run --rm simulator
   ```

   El comando imprime los identificadores aceptados. Cada ejecución usa identificadores nuevos. Que termine correctamente significa que Go aceptó los eventos; no que Scala haya calculado resultados.
3. Abre <http://localhost:8081/matches>. Verás una lista JSON con `matchId`, `summaryUrl` y `analysisUrl`. Antes de simular, la lista está vacía.
4. Copia una `summaryUrl` y agrégala a `http://localhost:8081` para ver juego, participantes, cantidad de eventos y estado de la partida. Haz lo mismo con `analysisUrl` para ver estado del análisis, intentos, error o resultado.

Para usar un identificador predecible (cámbialo al repetir en la misma sesión):

```sh
docker compose run --rm simulator -url http://go-service:8081 -prefix demo_01
```

Entonces abre:

- <http://localhost:8081/matches/demo_01_FREE_FIRE_1>
- <http://localhost:8081/matches/demo_01_FREE_FIRE_1/analysis>

`FINISHED` significa que Go cerró la partida. `COMPLETED` significa que Scala devolvió un resultado compatible. Con el Scala actual se espera `FAILED` y un error HTTP 404 para `/analyze`; no es un error de DNS ni prueba de que se perdieran los eventos.

## Qué se puede ver en Scala

- <http://localhost:8080/health>: salud del servidor Scala.
- <http://localhost:8080/analytics>: métricas de datos fijos de ejemplo, **no de las partidas enviadas por Go**.
- `http://localhost:8080/` puede seguir devolviendo 404 porque Scala no define una página inicial.

No hay una interfaz gráfica: las consultas muestran JSON. El antiguo proxy Go `/scala-metrics` se retiró; para ver ese mismo ejemplo utiliza directamente `/analytics` en el puerto 8080. El ejecutable Go actual es `cmd/server`, no el antiguo `go_service/main.go`.

## Cuando Scala implemente el contrato

Compose ya configura `SCALA_ANALYSIS_URL=http://scala-analytics:8080/analyze`. Reconstruye **solo Scala**, conservando el proceso Go y sus datos:

```sh
docker compose up -d --build --no-deps scala-analytics
```

Luego, en PowerShell, reintenta una partida fallida:

```powershell
Invoke-RestMethod -Method Post http://localhost:8081/matches/demo_01_FREE_FIRE_1/analysis/retry
Invoke-RestMethod http://localhost:8081/matches/demo_01_FREE_FIRE_1/analysis | ConvertTo-Json -Depth 20
```

También puedes crear nuevas partidas y esperar resultados compatibles:

```sh
docker compose run --rm simulator -url http://go-service:8081 -wait
```

No uses `-wait` como prueba exitosa del Scala actual: termina con error al detectar un análisis fallido. Los errores transitorios tienen reintentos automáticos; un 404 requiere corregir la API y reintentar manualmente.

## Detener y limitaciones

```sh
docker compose down
```

Los datos son **solo en memoria**: reiniciar o recrear Go elimina partidas, resultados y eventos. No hay base de datos ni volumen persistente. El healthcheck de Go no certifica disponibilidad ni compatibilidad de Scala.

La implementación Go cuenta con pruebas unitarias, HTTP y de concurrencia; sus pruebas de integración utilizan un servidor Scala simulado. La construcción y ejecución conjunta de los contenedores reales queda por verificar en un entorno con Docker; tampoco se han validado métricas reales de Scala con estos lotes.

Consulta el [README de Go](go_service/README.md) para arquitectura, variables, API, simulación local y pruebas. El [contrato Go–Scala](go_service/docs/scala_contract.md) describe los campos y respuestas pendientes de implementar en Scala.
