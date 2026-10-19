# Prueba local con Docker Desktop (Windows)

Ejecutar los comandos en **PowerShell**, desde la raíz del proyecto (la carpeta que contiene `docker-compose.yml`). Docker Desktop debe estar iniciado y utilizar contenedores Linux. No se necesita instalar Go, Java, Scala ni sbt.

## 1. Obtener la versión correcta

Antes de fusionar el PR, usar su rama:

```powershell
git fetch origin
git switch --track origin/feature/integrated-gamestats-release
```

Si la rama ya existe localmente, ejecutar `git switch feature/integrated-gamestats-release` y `git pull --ff-only`. Después del merge, usar `git switch main` y `git pull --ff-only`. Guardar los cambios locales propios antes de cambiar de rama.

## 2. Comprobar Docker y puertos

```powershell
docker info
docker compose version
docker ps
```

Los puertos 8080 y 8081 deben estar disponibles. Si siguen corriendo los contenedores de la versión anterior, detenerlos antes de iniciar la nueva:

```powershell
docker stop go-analytics-service scala-analytics-service
```

Ese comando solo corresponde si esos nombres aparecen en `docker ps`. Si otro proyecto ocupa los puertos, crear `.env` desde `.env.example` y cambiar `GO_PORT` y `SCALA_PORT`; usar esos puertos al abrir el navegador. El instalador obtiene los puertos efectivos para su prueba automática.

## 3. Construir, probar e iniciar

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install.ps1
```

El script crea las carpetas de datos, compila y prueba ambos módulos dentro de las imágenes, espera la salud de los servicios y ejecuta simulaciones por HTTP y archivos. La primera compilación puede tardar varios minutos.

Debe terminar mostrando `OK: Go y Scala integrados`. Comprobar:

```powershell
docker compose ps
```

Ambos servicios deben aparecer `healthy`. Si falla, guardar el error y consultar:

```powershell
docker compose logs --tail=150 go-service scala-analytics
```

No usar `SKIP_TESTS=true` para la primera validación de entrega. En Linux/macOS/WSL, ejecutar `bash scripts/install.sh` en lugar de PowerShell; el instalador también prepara los permisos de escritura de los volúmenes.

## 4. Consultar resultados

Con los puertos predeterminados:

- <http://localhost:8081/matches>: partidas recibidas en Go.
- <http://localhost:8080/results>: resultados calculados por Scala.
- <http://localhost:8080/stats/games>: estadísticas agregadas por juego.
- <http://localhost:8080/stats/games/FREE_FIRE/players>: historial por jugador.
- <http://localhost:8081/inbox>: procesamiento de archivos.

Se muestran documentos JSON; no hay interfaz gráfica. Los mensajes del simulador indican entrega de eventos; `COMPLETED` y las estadísticas consultadas confirman el análisis.

Para consultar una partida en PowerShell:

```powershell
$partidas = Invoke-RestMethod http://localhost:8081/matches
$id = $partidas.matches[0].matchId
Invoke-RestMethod "http://localhost:8081/matches/$id/analysis" | ConvertTo-Json -Depth 30
```

El resultado debe contener `status: COMPLETED`, estadísticas y un arreglo de patrones. No todas las partidas deben presentar patrones.

## 5. Generar nuevas partidas y probar archivos

```powershell
docker compose run --rm simulator -scenario random -matches 5 -wait
docker compose run --rm simulator -mode files -scenario random -matches 2 -wait
```

`-matches` indica la cantidad **por juego**: los comandos generan 20 y 8 partidas, respectivamente. Los identificadores predeterminados son nuevos en cada ejecución. En modo archivos, revisar `data/inbox/processed`, `data/inbox/rejected` y `data/results`.

Si un archivo se rechaza, consultar su reporte `*.error.json` en `rejected`. Un rechazo no significa que deba reiniciarse todo el proyecto.

## 6. Comprobar persistencia

```powershell
$antes = (Invoke-RestMethod http://localhost:8080/results).count
docker compose restart scala-analytics
```

Esperar a que `docker compose ps` muestre Scala `healthy` y comprobar:

```powershell
$despues = (Invoke-RestMethod http://localhost:8080/results).count
"Antes: $antes; después: $despues"
```

Los resultados de Scala se conservan en `data/results`. El estado de recepción y análisis de **Go es temporal** y se pierde al reiniciar su contenedor. Un resultado guardado sigue consultable en Scala aunque Go ya no tenga esa partida en memoria.

## 7. Detener y volver a iniciar

```powershell
docker compose down
docker compose up -d --wait
```

Los archivos del directorio `data` permanecen en el equipo. No es necesario borrarlos para volver a probar. Usar identificadores nuevos al enviar partidas diferentes.

## 8. Verificar la entrega

- Ambos contenedores `healthy`.
- Instalador y simulaciones con `-wait` sin errores.
- Resultados de los cuatro juegos en `/stats/games`.
- Partidas en `COMPLETED` y valores coherentes con sus eventos.
- Resultados persistidos y recuperados después del reinicio de Scala.
- Workflow CI/CD del PR finalizado correctamente.

La ejecución local mediante Compose es un entorno de despliegue reproducible. La verificación automática del pipeline corre en máquinas temporales de GitHub y no proporciona una dirección pública permanente.
