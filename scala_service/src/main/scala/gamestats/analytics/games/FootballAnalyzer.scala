package gamestats.analytics.games

import gamestats.analytics.{GameAnalyzer, Stats}
import gamestats.model.*
import gamestats.model.Actions.*

/** Fútbol: pases, goles, intercepciones y faltas; `data.teamId` identifica al equipo del actor. */
object FootballAnalyzer extends GameAnalyzer:
  val gameId: String = Games.Football

  val AssistWindowSeconds = 10.0
  val BuildUpMinPasses = 5
  val BuildUpWindowSeconds = 30.0
  val UnknownTeam = "UNKNOWN"

  private val completed: GameEvent => Boolean = _.bool("completed").contains(true)
  private val receiver: GameEvent => Option[String] = _.str("receiverPlayerId")
  private val target: GameEvent => Option[String] = _.str("targetPlayerId")

  /** Equipo de cada jugador según el primer evento propio que declara `teamId`. */
  def teams(ctx: MatchContext): Map[String, String] =
    ctx.playerActions
      .flatMap(event => event.str("teamId").map(event.playerId -> _))
      .foldLeft(Map.empty[String, String]) { case (acc, (player, team)) =>
        if acc.contains(player) then acc else acc.updated(player, team)
      }

  private def teamOf(event: GameEvent, teamByPlayer: Map[String, String]): String =
    event.str("teamId").orElse(teamByPlayer.get(event.playerId)).getOrElse(UnknownTeam)

  /** Asistencia: el último pase antes del gol fue completado hacia el goleador, dentro de 10 s. */
  def assists(ctx: MatchContext): List[(GameEvent, GameEvent)] =
    val passes = ctx.ofAction(Pass)
    for
      goal <- ctx.ofAction(Goal)
      scorer <- goal.actor.toList
      lastPass <- passes.filter(p => ctx.second(p) <= ctx.second(goal)).lastOption.toList
      if completed(lastPass) && receiver(lastPass).contains(scorer) && !lastPass.actor.contains(scorer)
      if ctx.second(goal) - ctx.second(lastPass) <= AssistWindowSeconds
    yield lastPass -> goal

  def playerStats(ctx: MatchContext): Map[String, ujson.Obj] =
    val teamByPlayer = teams(ctx)
    val passes = ctx.ofAction(Pass)
    val attempted = Stats.countBy(passes)(_.actor)
    val done = Stats.countBy(passes.filter(completed))(_.actor)
    val received = Stats.countBy(passes.filter(completed))(receiver)
    val goals = Stats.countBy(ctx.ofAction(Goal))(_.actor)
    val assistCount = Stats.countBy(assists(ctx).map(_._1))(_.actor)
    val interceptions = Stats.countBy(ctx.ofAction(Interception))(_.actor)
    val intercepted = Stats.countBy(ctx.ofAction(Interception))(target)
    val failedPasses = Stats.countBy(passes.filterNot(completed))(_.actor)
    val fouls = Stats.countBy(ctx.ofAction(Foul))(_.actor)
    val fouled = Stats.countBy(ctx.ofAction(Foul))(target)
    def get(counts: Map[String, Int], player: String) = counts.getOrElse(player, 0)

    ctx.participants.map { player =>
      player -> ujson.Obj(
        "team" -> teamByPlayer.getOrElse(player, UnknownTeam),
        "passesAttempted" -> get(attempted, player),
        "passesCompleted" -> get(done, player),
        "passAccuracyPct" -> Stats.pct(get(done, player), get(attempted, player)),
        "passesReceived" -> get(received, player),
        "goals" -> get(goals, player),
        "assists" -> get(assistCount, player),
        "goalContributions" -> (get(goals, player) + get(assistCount, player)),
        "interceptions" -> get(interceptions, player),
        "ballsLost" -> (get(intercepted, player) + get(failedPasses, player)),
        "foulsCommitted" -> get(fouls, player),
        "foulsReceived" -> get(fouled, player)
      )
    }.toMap

  def score(ctx: MatchContext): Map[String, Int] =
    val teamByPlayer = teams(ctx)
    val known = teamByPlayer.values.toList.distinct.map(_ -> 0).toMap
    known ++ Stats.countBy(ctx.ofAction(Goal))(goal => Some(teamOf(goal, teamByPlayer)))

  def matchStats(ctx: MatchContext): ujson.Obj =
    val teamByPlayer = teams(ctx)
    val goalsByTeam = score(ctx)
    val passesByTeam = ctx.ofAction(Pass).groupBy(teamOf(_, teamByPlayer))
    val totalPasses = ctx.ofAction(Pass).size
    val foulsByTeam = Stats.countBy(ctx.ofAction(Foul))(foul => Some(teamOf(foul, teamByPlayer)))
    val allTeams = (goalsByTeam.keySet ++ passesByTeam.keySet).toList.sorted

    val teamsJson = ujson.Obj.from(allTeams.map { team =>
      val teamPasses = passesByTeam.getOrElse(team, Nil)
      team -> ujson.Obj(
        "goals" -> goalsByTeam.getOrElse(team, 0),
        "passes" -> teamPasses.size,
        "completedPasses" -> teamPasses.count(completed),
        "passAccuracyPct" -> Stats.pct(teamPasses.count(completed), teamPasses.size),
        "possessionPct" -> Stats.pct(teamPasses.size, totalPasses),
        "fouls" -> foulsByTeam.getOrElse(team, 0)
      )
    })
    ujson.Obj(
      "score" -> Stats.countsJson(goalsByTeam),
      "teams" -> teamsJson,
      "totalGoals" -> ctx.ofAction(Goal).size,
      "totalPasses" -> totalPasses,
      "totalFouls" -> ctx.ofAction(Foul).size,
      "assistedGoals" -> assists(ctx).size,
      "result" -> Stats.uniqueMax(goalsByTeam.view.mapValues(_.toDouble).toMap).getOrElse("DRAW")
    )

  def inferredWinner(ctx: MatchContext): Option[String] =
    Stats.uniqueMax(score(ctx).view.mapValues(_.toDouble).toMap)

  def patterns(ctx: MatchContext): List[Pattern] =
    teamBuildUpGoals(ctx) ++ comeback(ctx).toList

  /** Gol de jugada colectiva: 5+ pases completados seguidos del mismo equipo en los 30 s previos, sin
    * intercepción, falta, pase fallado ni pase del rival en medio.
    */
  def teamBuildUpGoals(ctx: MatchContext): List[Pattern] =
    val teamByPlayer = teams(ctx)
    val ballEvents = ctx.ofAction(Pass, Interception, Foul, Goal)
    ctx.ofAction(Goal).flatMap { goal =>
      val team = teamOf(goal, teamByPlayer)
      val before = ballEvents.takeWhile(_.eventId != goal.eventId).reverse
      val chain = before.takeWhile { event =>
        event.is(Pass) && completed(event) && teamOf(event, teamByPlayer) == team &&
        ctx.second(goal) - ctx.second(event) <= BuildUpWindowSeconds
      }.reverse
      Option.when(chain.size >= BuildUpMinPasses && team != UnknownTeam) {
        Pattern(
          name = "TEAM_BUILDUP_GOAL",
          description = s"$team anotó tras ${chain.size} pases consecutivos completados",
          players = (chain.flatMap(_.actor) ++ goal.actor).distinct,
          eventIds = chain.map(_.eventId) :+ goal.eventId,
          startSecond = ctx.second(chain.head),
          endSecond = ctx.second(goal),
          details = ujson.Obj("team" -> team, "passes" -> chain.size)
        )
      }
    }

  /** Remontada: el equipo ganador estuvo abajo en el marcador en algún momento. */
  def comeback(ctx: MatchContext): Option[Pattern] =
    val teamByPlayer = teams(ctx)
    val goals = ctx.ofAction(Goal)
    for
      winnerTeam <- winner(ctx)
      // Marcador después de cada gol, construido con scanLeft sobre la secuencia de goles.
      timeline = goals.scanLeft(score(ctx).view.mapValues(_ => 0).toMap) { (board, goal) =>
        val team = teamOf(goal, teamByPlayer)
        board.updated(team, board.getOrElse(team, 0) + 1)
      }.tail.zip(goals)
      deficits = timeline.map { (board, goal) =>
        val own = board.getOrElse(winnerTeam, 0)
        val rival = board.removed(winnerTeam).values.maxOption.getOrElse(0)
        (rival - own, goal)
      }
      (worst, goal) <- deficits.maxByOption(_._1)
      if worst >= 1
    yield Pattern(
      name = "COMEBACK_VICTORY",
      description = s"$winnerTeam ganó después de ir perdiendo por $worst",
      players = Nil,
      eventIds = List(goal.eventId),
      startSecond = ctx.second(goal),
      endSecond = ctx.durationSeconds,
      details = ujson.Obj("team" -> winnerTeam, "maxDeficit" -> worst)
    )
