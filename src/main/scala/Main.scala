import model.*
import analytics.AggregatedMetrics
import com.sun.net.httpserver.{HttpServer, HttpHandler, HttpExchange}
import java.net.InetSocketAddress

@main def runServer(): Unit = {
  val server = HttpServer.create(new InetSocketAddress("0.0.0.0", 8080), 0)

  server.createContext("/health", new HttpHandler {
    override def handle(exchange: HttpExchange): Unit = {
      val response = """{"status": "ok", "service": "scala-analytics"}"""
      exchange.getResponseHeaders.set("Content-Type", "application/json")
      exchange.sendResponseHeaders(200, response.getBytes.length)
      val os = exchange.getResponseBody
      os.write(response.getBytes)
      os.close()
    }
  })

  server.createContext("/analytics", new HttpHandler {
    override def handle(exchange: HttpExchange): Unit = {
      val sampleEvents = List(
        GameEvent("e1", "2026-09-27T10:00:00Z", GameTypes.MMA, "m1", "p1", "ACTION", Actions.STRIKE_ATTEMPT),
        GameEvent("e2", "2026-09-27T10:00:01Z", GameTypes.MMA, "m1", "p1", "ACTION", Actions.STRIKE_LANDED),
        GameEvent("e3", "2026-09-27T10:00:02Z", GameTypes.MMA, "m1", "p1", "ACTION", Actions.STRIKE_ATTEMPT),
        GameEvent("e4", "2026-09-27T10:05:00Z", GameTypes.FREE_FIRE, "m2", "p1", "ACTION", Actions.DAMAGE_DEALT, Some(EventData(damage = Some(120.0)))),
        GameEvent("e5", "2026-09-27T10:05:05Z", GameTypes.FREE_FIRE, "m2", "p1", "ACTION", Actions.PLAYER_ELIMINATED, Some(EventData(victimId = Some("p2"))))
      )

      val mmaStats = AggregatedMetrics.mmaPrecisionRate(sampleEvents)
      val brStats = AggregatedMetrics.freeFirePlayerStats(sampleEvents)
      val p1Stats = brStats.getOrElse("p1", AggregatedMetrics.PlayerBRStats(0, 0))

      val mmaRatio = mmaStats.getOrElse("p1", 0.0)

      // Respuesta estructurada en formato JSON
      val response = s"""{
        |  "playerId": "p1",
        |  "mma": {
        |    "strikeAccuracyPercentage": $mmaRatio
        |  },
        |  "freeFire": {
        |    "kdRatio": ${p1Stats.kdRatio},
        |    "totalDamage": ${p1Stats.totalDamage}
        |  }
        |}""".stripMargin

      exchange.getResponseHeaders.set("Content-Type", "application/json")
      exchange.sendResponseHeaders(200, response.getBytes.length)
      val os = exchange.getResponseBody
      os.write(response.getBytes)
      os.close()
    }
  })

  server.setExecutor(null)
  server.start()
  println("Servidor HTTP Scala listo con JSON en el puerto 8080...")
  Thread.currentThread().join()
}