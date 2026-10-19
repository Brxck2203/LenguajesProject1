package gamestats

import com.sun.net.httpserver.{HttpExchange, HttpServer}
import gamestats.server.{Api, HttpRequest, HttpResponse, ResultStore}

import java.net.InetSocketAddress
import java.nio.charset.StandardCharsets
import java.nio.file.Path
import java.util.concurrent.{Executors, TimeUnit}
import scala.jdk.CollectionConverters.*
import scala.util.{Try, Using}

/** Borde imperativo del módulo Scala: lee la configuración, abre el puerto y traduce entre `HttpExchange` y la
  * función pura [[Api.handle]]. Todo el análisis ocurre fuera de este archivo.
  */
object Main:
  val MaxBodyBytes: Int = 16 * 1024 * 1024

  def main(args: Array[String]): Unit =
    val port = sys.env.get("PORT").flatMap(_.toIntOption).getOrElse(8080)
    val threads = sys.env.get("HTTP_THREADS").flatMap(_.toIntOption).filter(_ > 0).getOrElse(8)
    val resultsDir = sys.env.get("RESULTS_DIR").filter(_.trim.nonEmpty).map(Path.of(_))

    val store = ResultStore(resultsDir)
    val (loaded, invalid) = store.loadFromDisk()
    invalid.foreach(problem => println(s"[scala] resultado ignorado: $problem"))
    val api = Api(store)

    val pool = Executors.newFixedThreadPool(threads)
    val server = HttpServer.create(InetSocketAddress("0.0.0.0", port), 0)
    server.createContext("/", exchange => serve(api, exchange))
    server.setExecutor(pool)
    server.start()
    println(
      s"[scala] GameStats Analytics escuchando en :$port con $threads hilos; " +
        s"resultados en ${resultsDir.fold("memoria")(_.toString)} ($loaded cargados)"
    )

    sys.addShutdownHook {
      println("[scala] deteniendo servidor")
      server.stop(2)
      pool.shutdown()
      pool.awaitTermination(5, TimeUnit.SECONDS)
    }
    Thread.currentThread().join()

  private def serve(api: Api, exchange: HttpExchange): Unit =
    val response = Try(readBody(exchange)).toEither match
      case Left(_) => HttpResponse(400, ujson.Obj("error" -> "no se pudo leer el cuerpo"))
      case Right(None) => HttpResponse(413, ujson.Obj("error" -> s"el cuerpo supera $MaxBodyBytes bytes"))
      case Right(Some(body)) =>
        val headers = exchange.getRequestHeaders.asScala.toMap.collect {
          case (name, values) if !values.isEmpty => name -> values.get(0)
        }
        val request = HttpRequest(exchange.getRequestMethod, exchange.getRequestURI.getRawPath, headers, body)
        Try(api.handle(request)).fold(
          error => {
            println(s"[scala] error inesperado: $error")
            HttpResponse(500, ujson.Obj("error" -> "error interno del analizador"))
          },
          identity
        )
    write(exchange, response)

  private def readBody(exchange: HttpExchange): Option[String] =
    Using.resource(exchange.getRequestBody) { input =>
      // Se lee un byte más del límite para saber si el cuerpo lo supera.
      val bytes = input.readNBytes(MaxBodyBytes + 1)
      Option.when(bytes.length <= MaxBodyBytes)(String(bytes, StandardCharsets.UTF_8))
    }

  private def write(exchange: HttpExchange, response: HttpResponse): Unit =
    val bytes = ujson.write(response.body, indent = 2).getBytes(StandardCharsets.UTF_8)
    exchange.getResponseHeaders.set("Content-Type", "application/json; charset=utf-8")
    response.headers.foreach((name, value) => exchange.getResponseHeaders.set(name, value))
    exchange.sendResponseHeaders(response.status, bytes.length.toLong)
    Using.resource(exchange.getResponseBody)(_.write(bytes))
