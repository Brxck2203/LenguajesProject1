package gamestats.analytics

import gamestats.model.AnalysisResult

/** Estadísticas que combinan varias partidas ya analizadas (por juego y por jugador a lo largo del tiempo).
  *
  * Son genéricas: recorren los campos numéricos de `playerStats` de cualquier juego y calculan total y promedio con
  * `foldLeft`, sin conocer los nombres de las métricas.
  */
object CrossMatchStats:

  private def number(value: ujson.Value): Option[Double] = value match
    case ujson.Num(n) => Some(n)
    case _            => None

  private def field(obj: ujson.Obj, name: String): Option[ujson.Value] = obj.value.get(name)

  def gamesSummary(results: List[AnalysisResult]): ujson.Obj =
    ujson.Obj.from(results.groupBy(_.gameId).toList.sortBy(_._1).map { (gameId, games) =>
      val durations = games.flatMap(r => field(r.matchStats, "durationSeconds").flatMap(number))
      val events = games.flatMap(r => field(r.matchStats, "eventCount").flatMap(number)).sum
      val patternCounts = games.flatMap(_.patterns.map(_.name)).groupMapReduce(identity)(_ => 1)(_ + _)
      val winners = games.flatMap(r => field(r.matchStats, "winner")).collect { case ujson.Str(w) => w }
      gameId -> ujson.Obj(
        "matchesAnalyzed" -> games.size,
        "totalEvents" -> events,
        "averageDurationSeconds" -> Stats.optional(Stats.mean(durations))(v => ujson.Num(Stats.round2(v))),
        "averageEventsPerMatch" -> Stats.round2(events / games.size),
        "patternCounts" -> Stats.countsJson(patternCounts),
        "topWinners" -> ujson.Arr.from(
          winners.groupMapReduce(identity)(_ => 1)(_ + _).toList.sortBy((w, n) => (-n, w)).take(5).map((w, n) =>
            ujson.Obj("winner" -> w, "wins" -> n)
          )
        )
      )
    })

  /** Historial agregado de cada jugador dentro de un juego, ordenado por victorias y partidas. */
  def playersSummary(results: List[AnalysisResult], gameId: String): ujson.Arr =
    val games = results.filter(_.gameId == gameId)
    val appearances = games.flatMap(r => r.playerStats.toList.map((player, stats) => (player, r, stats)))
    val byPlayer = appearances.groupBy(_._1)

    val rows = byPlayer.toList.map { (player, rows) =>
      val stats = rows.map(_._3)
      val wins = stats.count(s => field(s, "isWinner").contains(ujson.True))
      // Suma por nombre de métrica: fold sobre todos los campos numéricos de todas las partidas del jugador.
      val totals = stats.foldLeft(Map.empty[String, (Double, Int)]) { (acc, s) =>
        s.value.foldLeft(acc) { case (inner, (name, value)) =>
          number(value).fold(inner) { n =>
            val (sum, count) = inner.getOrElse(name, (0.0, 0))
            inner.updated(name, (sum + n, count + 1))
          }
        }
      }
      val metrics = ujson.Obj.from(totals.toList.sortBy(_._1).map { case (name, (sum, count)) =>
        name -> ujson.Obj("total" -> Stats.round2(sum), "average" -> Stats.round2(sum / count))
      })
      (player, rows.size, wins, metrics, rows.map(_._2.matchId).sorted)
    }

    ujson.Arr.from(rows.sortBy((player, matches, wins, _, _) => (-wins, -matches, player)).map {
      (player, matches, wins, metrics, matchIds) =>
        ujson.Obj(
          "playerId" -> player,
          "matchesPlayed" -> matches,
          "wins" -> wins,
          "winRatePct" -> Stats.pct(wins, matches),
          "metrics" -> metrics,
          "matchIds" -> ujson.Arr.from(matchIds)
        )
    })
