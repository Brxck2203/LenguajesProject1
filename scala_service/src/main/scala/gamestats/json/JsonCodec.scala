package gamestats.json

import gamestats.model.*

import java.time.{Instant, OffsetDateTime}
import scala.util.Try

/** Decodificación del lote enviado por Go.
  *
  * Toda la validación devuelve `Either[String, A]`: el primer error encontrado se propaga sin excepciones y sin
  * estado mutable, componiendo las validaciones con `for`/`flatMap`.
  */
object JsonCodec:

  type Decoded[A] = Either[String, A]

  def decodeRequest(body: String): Decoded[AnalysisRequest] =
    for
      json <- Try(ujson.read(body)).toEither.left.map(error => s"JSON inválido: ${error.getMessage}")
      root <- asObject(json, "cuerpo")
      matchId <- requiredString(root, "matchId")
      gameId <- requiredString(root, "gameId")
      rawEvents <- root.get("events").collect { case ujson.Arr(values) => values.toList }
        .toRight("events debe ser un arreglo JSON")
      events <- traverse(rawEvents.zipWithIndex) { (value, index) =>
        decodeEvent(value).left.map(error => s"events[$index]: $error")
      }
      request <- validateBatch(AnalysisRequest(matchId, gameId, events))
    yield request

  def decodeEvent(json: ujson.Value): Decoded[GameEvent] =
    for
      obj <- asObject(json, "evento")
      eventId <- requiredString(obj, "eventId")
      rawTimestamp <- requiredString(obj, "timestamp")
      timestamp <- parseInstant(rawTimestamp)
      gameId <- requiredString(obj, "gameId")
      matchId <- requiredString(obj, "matchId")
      playerId <- requiredString(obj, "playerId")
      eventType <- requiredString(obj, "type")
      action <- requiredString(obj, "action")
      data <- obj.get("data") match
        case Some(ujson.Obj(fields)) => Right(fields.toMap)
        case None | Some(ujson.Null) => Right(Map.empty[String, ujson.Value])
        case Some(_)                 => Left("data debe ser un objeto JSON")
    yield GameEvent(eventId, timestamp, gameId, matchId, playerId, eventType, action, data)

  /** Reglas del contrato: una sola partida, con inicio y fin, y sin eventos repetidos. */
  private def validateBatch(request: AnalysisRequest): Decoded[AnalysisRequest] =
    val foreign = request.events.filter(e => e.matchId != request.matchId || e.gameId != request.gameId)
    val duplicated = request.events.groupBy(_.eventId).collect { case (id, list) if list.size > 1 => id }
    if request.events.isEmpty then Left("el lote no contiene eventos")
    else if foreign.nonEmpty then Left(s"el evento ${foreign.head.eventId} pertenece a otra partida o juego")
    else if duplicated.nonEmpty then Left(s"eventId repetido en el lote: ${duplicated.toList.sorted.head}")
    else if !request.events.exists(_.is(EventTypes.MatchStarted)) then Left("el lote no contiene MATCH_STARTED")
    else if !request.events.exists(_.is(EventTypes.MatchFinished)) then Left("el lote no contiene MATCH_FINISHED")
    else Right(request)

  /** Convierte una lista de resultados en un resultado de lista, deteniéndose en el primer error. */
  def traverse[A, B](values: List[A])(f: A => Decoded[B]): Decoded[List[B]] =
    values.foldRight[Decoded[List[B]]](Right(Nil)) { (value, acc) =>
      for
        head <- f(value)
        tail <- acc
      yield head :: tail
    }

  private def asObject(json: ujson.Value, what: String): Decoded[Map[String, ujson.Value]] =
    json match
      case ujson.Obj(fields) => Right(fields.toMap)
      case _                 => Left(s"$what debe ser un objeto JSON")

  private def requiredString(obj: Map[String, ujson.Value], field: String): Decoded[String] =
    obj.get(field) match
      case Some(ujson.Str(value)) if value.trim.nonEmpty => Right(value)
      case _                                            => Left(s"$field es obligatorio y debe ser texto no vacío")

  /** RFC 3339 con zona `Z` u offset (`-06:00`) y fracciones de segundo opcionales. */
  private def parseInstant(raw: String): Decoded[Instant] =
    Try(OffsetDateTime.parse(raw).toInstant).toEither.left.map(_ => s"timestamp no es RFC 3339: $raw")
