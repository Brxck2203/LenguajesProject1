import cask.Request
import model.*
import analytics.{AggregatedMetrics, PatternDetection}
import scala.collection.mutable.ListBuffer

object AnalyticsServer extends cask.MainRoutes:

  // Almacenamiento en memoria para los eventos recibidos
  private val eventLog = ListBuffer[GameEvent]()

  // Endpoint para recibir e ingestar eventos en JSON
  @cask.post("/events")
  def receiveEvent(request: Request) =
    try
      val jsonStr = request.text()
      val event = GameEvent.fromJson(jsonStr)
      eventLog.append(event)
      cask.Response(
        data = ujson.Obj("status" -> "success", "eventId" -> event.eventId).render(),
        statusCode = 200,
        headers = Seq("Content-Type" -> "application/json")
      )
    catch
      case e: Exception =>
        cask.Response(
          data = ujson.Obj("status" -> "error", "message" -> e.getMessage).render(),
          statusCode = 400,
          headers = Seq("Content-Type" -> "application/json")
        )

  // Endpoint para consultar los patrones detectados
  @cask.get("/patterns")
  def getPatterns() =
    val events = eventLog.toList
    val thirdParty = PatternDetection.detectThirdPartyEliminations(events)
    val tikiTaka   = PatternDetection.detectTikiTakaGoals(events)
    
    cask.Response(
      data = ujson.Obj(
        "thirdPartyEliminations" -> thirdParty,
        "tikiTakaGoals"          -> tikiTaka
      ).render(),
      statusCode = 200,
      headers = Seq("Content-Type" -> "application/json")
    )

  // Endpoint de estado/salud
  @cask.get("/health")
  def health() =
    cask.Response(
      data = ujson.Obj("status" -> "UP", "totalEvents" -> eventLog.length).render(),
      statusCode = 200,
      headers = Seq("Content-Type" -> "application/json")
    )

  initialize()