package gamestats.server

import gamestats.json.ResultCodec
import gamestats.model.AnalysisResult

import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Path, StandardCopyOption}
import java.security.MessageDigest
import java.time.Instant
import java.util.concurrent.atomic.AtomicReference
import scala.jdk.CollectionConverters.*
import scala.util.{Try, Using}

final case class StoredResult(bodyHash: String, analyzedAt: Instant, result: AnalysisResult):
  def toJson: ujson.Obj = ujson.Obj(
    "bodyHash" -> bodyHash,
    "analyzedAt" -> analyzedAt.toString,
    "result" -> result.toJson
  )

enum RegisterOutcome:
  case Created(stored: StoredResult)
  case AlreadyExists(stored: StoredResult)
  case Conflict(existing: StoredResult)

/** Resultados por `matchId`, seguros para peticiones concurrentes.
  *
  * El estado es un `Map` inmutable dentro de un `AtomicReference`: cada registro produce un mapa nuevo con una
  * operación atómica, así no se comparten colecciones mutables entre hilos. Si se configura un directorio, cada
  * resultado se guarda además como JSON (`<matchId>.json`) y se vuelve a cargar al iniciar.
  */
final class ResultStore(directory: Option[Path]):
  private val state = AtomicReference(Map.empty[String, StoredResult])

  def get(matchId: String): Option[StoredResult] = state.get().get(matchId)

  def all: List[StoredResult] = state.get().values.toList.sortBy(s => (s.analyzedAt, s.result.matchId))

  /** Registra un resultado aplicando las reglas de idempotencia del contrato. */
  def register(candidate: StoredResult): RegisterOutcome =
    val matchId = candidate.result.matchId
    val before = state.getAndUpdate { current =>
      if current.contains(matchId) then current else current.updated(matchId, candidate)
    }
    before.get(matchId) match
      case None =>
        persist(candidate)
        RegisterOutcome.Created(candidate)
      case Some(existing) if existing.bodyHash == candidate.bodyHash => RegisterOutcome.AlreadyExists(existing)
      case Some(existing)                                            => RegisterOutcome.Conflict(existing)

  /** Carga los resultados guardados previamente; devuelve cuántos se cargaron y los archivos inválidos. */
  def loadFromDisk(): (Int, List[String]) =
    directory.filter(Files.isDirectory(_)).fold((0, Nil)) { dir =>
      val files = Using.resource(Files.list(dir))(_.iterator().asScala.toList)
        .filter(_.getFileName.toString.endsWith(".json"))
      val decoded = files.map { file =>
        // All access to the stored envelope belongs inside Try: valid JSON may
        // still lack fields, contain wrong types or have an invalid timestamp.
        val envelope = Try {
          val json = ujson.read(Files.readString(file, StandardCharsets.UTF_8))
          (json("result"), json("bodyHash").str, Instant.parse(json("analyzedAt").str))
        }.toEither.left.map(_.getMessage)
        val parsed = envelope.flatMap { (rawResult, hash, analyzedAt) =>
          ResultCodec.decodeResult(rawResult).map(result => StoredResult(hash, analyzedAt, result))
        }
        parsed.left.map(error => s"${file.getFileName}: $error")
      }
      val loaded = decoded.collect { case Right(stored) => stored }
      state.updateAndGet(current => current ++ loaded.map(s => s.result.matchId -> s))
      (loaded.size, decoded.collect { case Left(error) => error })
    }

  /** Un fallo de disco no invalida el análisis: el resultado sigue disponible en memoria y se informa en el log. */
  private def persist(stored: StoredResult): Unit =
    directory.foreach { dir =>
      Try {
        Files.createDirectories(dir)
        val target = dir.resolve(ResultStore.fileName(stored.result.matchId))
        val temporary = Files.createTempFile(dir, ".result-", ".tmp")
        Files.writeString(temporary, ujson.write(stored.toJson, indent = 2), StandardCharsets.UTF_8)
        Files.move(temporary, target, StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE)
      }.failed.foreach(error => println(s"[scala] no se pudo guardar ${stored.result.matchId}: $error"))
    }

object ResultStore:
  def sha256(text: String): String =
    MessageDigest.getInstance("SHA-256").digest(text.getBytes(StandardCharsets.UTF_8)).map("%02x".format(_)).mkString

  /** Nombre de archivo seguro; si hubo que reemplazar caracteres se agrega un hash para evitar colisiones. */
  def fileName(matchId: String): String =
    val safe = matchId.map(c => if c.isLetterOrDigit || c == '-' || c == '_' || c == '.' then c else '_')
    if safe == matchId then s"$safe.json" else s"${safe}_${sha256(matchId).take(8)}.json"
