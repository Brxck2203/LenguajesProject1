# Scala 3 & Go Analytics Microservices Platform

Este proyecto es una plataforma de microservicios orquestada con **Docker Compose** que procesa eventos de videojuegos (MMA y Free Fire) para calcular métricas agregadas y exponerlas mediante un proxy HTTP.

---

## 🛠️ Arquitectura y Estado del Proyecto

| Componente | Tecnología | Puerto Interno / Externo | Estado | Descripción |
| :--- | :--- | :---: | :---: | :--- |
| **Scala Analytics Service** | Scala 3.3 / Java JDK HttpServer | `8080:8080` | **100% Completado** | Procesa los eventos de juego, calcula métricas agregadas y expone una API REST en JSON con soporte CORS. |
| **Go Proxy Service** | Go 1.22 | `8081:8081` | **Estructura Base / En Proceso** | Actúa como gateway/proxy que consulta internamente al servicio de Scala vía DNS de Docker. |
| **Orquestación** | Docker Compose | N/A | **100% Completado** | Mantiene la red interna `analytics-net` e interconecta los contenedores. |

---

## ⚙️ Lo que se ha realizado (Módulo Scala 3 & Docker)

1. **Cálculo de Métricas Agregadas (`AggregatedMetrics`)**:
   - **MMA (`mmaPrecisionRate`)**: Proporción de golpes conectados (`STRIKE_LANDED`) sobre intentos (`STRIKE_ATTEMPT`)[cite: 3].
   - **Free Fire (`freeFirePlayerStats`)**: Acumulado de daño total y proporción K/D (Kills/Deaths)[cite: 3].
2. **Servidor HTTP Persistente (`src/main/scala/Main.scala`)**:
   - Servidor nativo basado en el JDK (`com.sun.net.httpserver.HttpServer`) para evitar incompatibilidades de dependencias[cite: 3].
   - Bloqueo de hilo con `Thread.currentThread().join()` para asegurar la persistencia en Docker[cite: 3].
   - Cabeceras **CORS** y soporte para preflight (`OPTIONS`) habilitados para peticiones externas[cite: 3].
3. **Contrato de API REST en JSON (`http://scala-analytics:8080/analytics`)**:
   ```json
   {
     "playerId": "p1",
     "mma": {
       "strikeAccuracyPercentage": 50.0
     },
     "freeFire": {
       "kdRatio": 1.0,
       "totalDamage": 120.0
     }
   }

```

---

## 📋 Tareas Pendientes para el Servicio en Go (`go_service/`)

* [ ] **Desarrollar la lógica de negocio final en Go** (`go_service/main.go`).
* [ ] **Consumir la API de Scala** desde el puerto interno enviando peticiones `GET` a `http://scala-analytics:8080/analytics`.


* [ ] **Deserializar el JSON** con la estructura correspondiente (`json.Unmarshal`).


* [ ] **Persistencia / Base de Datos** (si el proyecto requiere almacenar los resultados en una BD).
* [ ] **Exponer los endpoints finales** para los clientes o la entrega del proyecto.

---

## 🚀 Comandos para Ejecutar el Proyecto

### Requisitos previos

* Tener **Docker Desktop** iniciado y en ejecución.

### 1. Iniciar la infraestructura completa

Abre una terminal de PowerShell en la raíz del proyecto y ejecuta:

```powershell
docker compose up --build

```

### 2. Probar los endpoints desde una segunda terminal

```powershell
# Probar la salud del servicio de Scala
Invoke-RestMethod -Uri "http://localhost:8080/health" -Method Get

# Obtener las métricas JSON directo desde Scala
Invoke-RestMethod -Uri "http://localhost:8080/analytics" -Method Get

# Obtener las métricas a través del Proxy en Go
(Invoke-RestMethod -Uri "http://localhost:8081/scala-metrics" -Method Get) | ConvertTo-Json -Depth 5

```

### 3. Detener los contenedores

```powershell
docker compose down

```

``
<FollowUp label="¿Quieres que guardemos este contenido directamente en un archivo README.md usando PowerShell?" query="Ejecuta el comando para crear o sobrescribir el archivo README.md con la documentación generada."/>
