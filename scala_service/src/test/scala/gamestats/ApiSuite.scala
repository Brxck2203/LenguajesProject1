package gamestats

import gamestats.TestData.*
import gamestats.json.JsonCodec
import gamestats.model.*
import gamestats.model.Actions.*
import gamestats.server.*

import java.nio.file.Files
import java.nio.file.attribute.PosixFilePermission

class JsonCodecSuite extends munit.FunSuite:
  private val valid = MatchBuilder(Games.FreeFire, "ff1").start
    .at(1, DamageDealt, "p1", "targetPlayerId" -> "p2", "damage" -> 50)
    .finish(5)
    .request

  test("decodifica el lote que envía Go") {
    val decoded = JsonCodec.decodeRequest(toJson(valid))
    assertEquals(decoded.map(_.events.size), Right(3))
    assertEquals(decoded.map(_.events(1).num("damage")), Right(Some(50.0)))
  }

  test("acepta timestamps con offset y fracciones de segundo") {
    val body = toJson(valid).replace("2026-10-01T12:00:01Z", "2026-10-01T06:00:01.250-06:00")
    assertEquals(JsonCodec.decodeRequest(body).map(_.events(1).timestamp.toString), Right("2026-10-01T12:00:01.250Z"))
  }

  test("rechaza lotes incompletos o mezclados") {
    val withoutFinish = valid.copy(events = valid.events.dropRight(1))
    assert(JsonCodec.decodeRequest(toJson(withoutFinish)).left.exists(_.contains("MATCH_FINISHED")))
    val foreign = valid.copy(events = valid.events.map(_.copy(matchId = "otra")))
    assert(JsonCodec.decodeRequest(toJson(foreign)).left.exists(_.contains("otra partida")))
    assert(JsonCodec.decodeRequest("{no es json").isLeft)
    assert(JsonCodec.decodeRequest("""{"matchId":"x","gameId":"MMA","events":{}}""").isLeft)
  }

  test("informa el índice del evento inválido") {
    val body = toJson(valid).replaceFirst("\"playerId\":\"p1\"", "\"playerId\":\"\"")
    assertEquals(JsonCodec.decodeRequest(body).left.map(_.take(10)), Left("events[1]:"))
  }

class ApiSuite extends munit.FunSuite:
  private val batch = MatchBuilder(Games.Mma, "mma1").start
    .at(1, StrikeLanded, "a", "targetPlayerId" -> "b")
    .finish(10)
    .request
  private val json = Map("Content-Type" -> "application/json")

  private def post(api: Api, body: String, headers: Map[String, String] = json) =
    api.handle(HttpRequest("POST", "/analyze", headers, body))
  private def get(api: Api, path: String) = api.handle(HttpRequest("GET", path, Map.empty, ""))

  test("POST /analyze devuelve el contrato completo") {
    val api = Api(ResultStore(None))
    val response = post(api, toJson(batch), json + ("Idempotency-Key" -> "abc"))
    assertEquals(response.status, 200)
    assertEquals(response.body("matchId").str, "mma1")
    assert(response.body("playerStats").obj.contains("a"))
    assert(response.body("matchStats").obj.contains("durationSeconds"))
    assertEquals(response.body("patterns").arr.size, 0)
    assertEquals(response.headers.get("Idempotency-Key"), Some("abc"))
  }

  test("repetir el mismo lote reutiliza el resultado; un lote distinto es conflicto") {
    val api = Api(ResultStore(None))
    assertEquals(post(api, toJson(batch)).status, 200)
    val replay = post(api, toJson(batch))
    assertEquals(replay.status, 200)
    assertEquals(replay.headers.get("Idempotent-Replay"), Some("true"))
    val changed = batch.copy(events = batch.events.map(e => if e.is(StrikeLanded) then e.copy(playerId = "b") else e))
    assertEquals(post(api, toJson(changed)).status, 409)
    assertEquals(get(api, "/results").body("count").num, 1.0)
  }

  test("errores HTTP: 415, 400, 404 y 405") {
    val api = Api(ResultStore(None))
    assertEquals(post(api, toJson(batch), Map("Content-Type" -> "text/plain")).status, 415)
    assertEquals(post(api, "{}").status, 400)
    assertEquals(get(api, "/results/nada").status, 404)
    assertEquals(get(api, "/no-existe").status, 404)
    val wrongMethod = get(api, "/analyze")
    assertEquals(wrongMethod.status, 405)
    assertEquals(wrongMethod.headers.get("Allow"), Some("POST"))
  }

  test("estadísticas entre partidas por juego y por jugador") {
    val api = Api(ResultStore(None))
    val second = MatchBuilder(Games.Mma, "mma2").start
      .at(1, StrikeLanded, "a", "targetPlayerId" -> "c")
      .at(2, StrikeLanded, "a", "targetPlayerId" -> "c")
      .finish(20)
      .request
    post(api, toJson(batch))
    post(api, toJson(second))
    val games = get(api, "/stats/games").body
    assertEquals(games("MMA")("matchesAnalyzed").num, 2.0)
    val players = get(api, "/stats/games/MMA/players").body("players").arr
    val a = players.find(_("playerId").str == "a").get
    assertEquals(a("matchesPlayed").num, 2.0)
    assertEquals(a("wins").num, 2.0)
    assertEquals(a("metrics")("strikesLanded")("total").num, 3.0)
    assertEquals(a("metrics")("strikesLanded")("average").num, 1.5)
  }

  test("los resultados se guardan en disco y se recuperan al reiniciar") {
    val dir = Files.createTempDirectory("gamestats-results")
    val first = Api(ResultStore(Some(dir)))
    assertEquals(post(first, toJson(batch)).status, 200)
    val persisted = dir.resolve("mma1.json")
    assert(Files.exists(persisted))
    if Files.getFileStore(persisted).supportsFileAttributeView("posix") then
      val permissions = Files.getPosixFilePermissions(persisted)
      assert(permissions.contains(PosixFilePermission.GROUP_READ))
      assert(permissions.contains(PosixFilePermission.OTHERS_READ))

    val restarted = ResultStore(Some(dir))
    assertEquals(restarted.loadFromDisk(), (1, Nil))
    val api = Api(restarted)
    assertEquals(get(api, "/results/mma1").status, 200)
    // Tras reiniciar, el mismo lote sigue siendo idempotente.
    assertEquals(post(api, toJson(batch)).headers.get("Idempotent-Replay"), Some("true"))
  }

  test("reinicia con resultados válidos e ignora archivos JSON corruptos o incompletos") {
    val dir = Files.createTempDirectory("gamestats-mixed-results")
    val first = Api(ResultStore(Some(dir)))
    assertEquals(post(first, toJson(batch)).status, 200)

    val valid = ujson.read(Files.readString(dir.resolve("mma1.json")))
    val invalidFiles = Map(
      "empty-envelope.json" -> "{}",
      "invalid-json.json" -> "{broken",
      "null-envelope.json" -> "null",
      "invalid-result.json" -> ujson.write(ujson.Obj(
        "bodyHash" -> "hash", "analyzedAt" -> "2026-10-01T12:00:00Z", "result" -> ujson.Obj()
      )),
      "invalid-hash.json" -> ujson.write(ujson.Obj(
        "bodyHash" -> 42, "analyzedAt" -> valid("analyzedAt"), "result" -> valid("result")
      )),
      "invalid-time.json" -> ujson.write(ujson.Obj(
        "bodyHash" -> "hash", "analyzedAt" -> "not-a-date", "result" -> valid("result")
      ))
    )
    invalidFiles.foreach((name, body) => Files.writeString(dir.resolve(name), body))
    val restarted = ResultStore(Some(dir))
    val (loaded, errors) = restarted.loadFromDisk()
    assertEquals(loaded, 1)
    assertEquals(errors.size, invalidFiles.size)
    invalidFiles.keys.foreach(name => assert(errors.exists(_.startsWith(name + ":"))))
    val api = Api(restarted)
    assertEquals(get(api, "/health").status, 200)
    assertEquals(get(api, "/results/mma1").status, 200)
    assertEquals(post(api, toJson(batch)).headers.get("Idempotent-Replay"), Some("true"))
  }

  test("nombres de archivo seguros") {
    assertEquals(ResultStore.fileName("demo_01"), "demo_01.json")
    assert(ResultStore.fileName("../x").startsWith(".._x_"))
  }
