package gamestats.analytics

import gamestats.model.GameEvent

/** Funciones puras reutilizadas por todos los analizadores. */
object Stats:

  def round2(value: Double): Double = math.round(value * 100.0) / 100.0

  /** Porcentaje `part / total`, 0 cuando no hay total. */
  def pct(part: Int, total: Int): Double =
    if total <= 0 then 0.0 else round2(part.toDouble / total * 100.0)

  def ratio(numerator: Double, denominator: Double): Double =
    if denominator == 0 then round2(numerator) else round2(numerator / denominator)

  def mean(values: Seq[Double]): Option[Double] =
    Option.when(values.nonEmpty)(values.sum / values.size)

  def stdDev(values: Seq[Double]): Option[Double] =
    mean(values).map { avg =>
      math.sqrt(values.map(v => math.pow(v - avg, 2)).sum / values.size)
    }

  /** Cuenta eventos agrupándolos por una clave opcional (por ejemplo actor u objetivo). */
  def countBy(events: Seq[GameEvent])(key: GameEvent => Option[String]): Map[String, Int] =
    events.flatMap(key).groupMapReduce(identity)(_ => 1)(_ + _)

  /** Suma un valor numérico de `data` agrupando por una clave. */
  def sumBy(events: Seq[GameEvent])(key: GameEvent => Option[String], value: GameEvent => Option[Double])
      : Map[String, Double] =
    events
      .flatMap(event => key(event).zip(value(event)))
      .groupMapReduce(_._1)(_._2)(_ + _)

  /** Clave máxima cuando es única (sin empate). */
  def uniqueMax[K](scores: Map[K, Double]): Option[K] =
    scores.maxByOption(_._2).filter { case (_, best) => scores.values.count(_ == best) == 1 }.map(_._1)

  /** Convierte un mapa de conteos a JSON con claves ordenadas (resultado determinista). */
  def countsJson(counts: Map[String, Int]): ujson.Obj =
    ujson.Obj.from(counts.toList.sortBy(_._1).map((key, count) => key -> ujson.Num(count)))

  def optional[A](value: Option[A])(toJson: A => ujson.Value): ujson.Value =
    value.fold[ujson.Value](ujson.Null)(toJson)

  /** Une dos objetos JSON; los campos de `extra` prevalecen. */
  def merge(base: ujson.Obj, extra: ujson.Obj): ujson.Obj =
    ujson.Obj.from(base.value.toList ++ extra.value.toList)
