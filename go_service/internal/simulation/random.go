package simulation

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

// Player pools are shared by every generated match so the cross-match statistics in
// Scala (wins, totals and averages per player) accumulate real history.
var (
	fighterPool = []string{"ff_ana_rios", "ff_bruno_mora", "ff_carla_vega", "ff_diego_solis", "ff_elena_cruz", "ff_fabio_leon", "ff_gina_paz", "ff_hugo_rojas"}
	teamPool    = []string{"LEONES", "TIGRES", "AGUILAS", "TOROS"}
	riderPool   = []string{"rd_alvarez", "rd_bianchi", "rd_castro", "rd_duarte", "rd_espinoza", "rd_fonseca", "rd_gomez", "rd_herrera", "rd_ibarra", "rd_jimenez"}
	weaponPool  = []string{"AR", "SMG", "SNIPER", "SHOTGUN", "PISTOL"}
)

func battleRoyalePool() []string {
	players := make([]string, 20)
	for i := range players {
		players[i] = fmt.Sprintf("br_player_%02d", i+1)
	}
	return players
}

// RandomConfig controls the random generator. The same Seed always produces the same matches.
type RandomConfig struct {
	Prefix         string
	Seed           uint64
	MatchesPerGame int
	Start          time.Time
}

// builder accumulates the events of one match with simulated timestamps.
type builder struct {
	match Match
	game  string
	base  time.Time
}

func newBuilder(id, game, scenario string, base time.Time) *builder {
	return &builder{match: Match{ID: id, Scenario: scenario}, game: game, base: base}
}

func (b *builder) add(seconds float64, action, actor string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	raw, _ := json.Marshal(data)
	typ := domain.TypeAction
	if action == domain.ActionMatchStarted || action == domain.ActionMatchFinished {
		typ = domain.TypeMatch
	}
	b.match.Events = append(b.match.Events, domain.Event{
		EventID:   fmt.Sprintf("%s_evt_%04d", b.match.ID, len(b.match.Events)),
		Timestamp: b.base.Add(time.Duration(seconds * float64(time.Second))),
		GameID:    b.game, MatchID: b.match.ID, PlayerID: actor, Type: typ, Action: action, Data: raw,
	})
}

func (b *builder) done() Match {
	sort.SliceStable(b.match.Events, func(i, j int) bool { return b.match.Events[i].Timestamp.Before(b.match.Events[j].Timestamp) })
	return b.match
}

// Random generates MatchesPerGame matches for each of the four games.
func Random(config RandomConfig) []Match {
	if config.MatchesPerGame <= 0 {
		config.MatchesPerGame = 3
	}
	if config.Start.IsZero() {
		config.Start = time.Now().UTC().Truncate(time.Second)
	}
	var matches []Match
	for n := 0; n < config.MatchesPerGame; n++ {
		for g, generate := range []func(string, *rand.Rand, time.Time) Match{randomMMA, randomFootball, randomRace, randomBattleRoyale} {
			rng := rand.New(rand.NewPCG(config.Seed, uint64(n*10+g+1)))
			base := config.Start.Add(time.Duration(n) * time.Hour)
			id := fmt.Sprintf("%s_r%02d_%d", config.Prefix, n+1, g+1)
			matches = append(matches, generate(id, rng, base))
		}
	}
	return matches
}

func pick[T any](rng *rand.Rand, values []T) T { return values[rng.IntN(len(values))] }

func sample(rng *rand.Rand, values []string, n int) []string {
	shuffled := append([]string(nil), values...)
	rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	return shuffled[:n]
}

// randomMMA: three 5-minute rounds; accuracy and aggression differ per fighter and a run
// of landed strikes raises the knockdown probability, so flurries and comebacks appear.
func randomMMA(id string, rng *rand.Rand, base time.Time) Match {
	fighters := sample(rng, fighterPool, 2)
	b := newBuilder(id, domain.GameMMA, "random MMA bout", base)
	accuracy := map[string]float64{fighters[0]: 0.3 + rng.Float64()*0.3, fighters[1]: 0.3 + rng.Float64()*0.3}
	aggression := 0.4 + rng.Float64()*0.2
	landed := map[string]int{}
	var recent []struct {
		at      float64
		fighter string
	}
	b.add(0, domain.ActionMatchStarted, domain.SystemPlayerID, map[string]any{"mode": "3x5min"})
	t, winner, reason := 0.0, "", "DECISION"
	attacker, defender, combo := fighters[0], fighters[1], false
	for t < 900 {
		if combo {
			// A landed strike is often followed by a quick combination from the same fighter.
			t += 0.4 + rng.Float64()*0.8
		} else {
			t += 2 + rng.Float64()*6
			attacker, defender = fighters[0], fighters[1]
			if rng.Float64() > aggression {
				attacker, defender = defender, attacker
			}
			if rng.Float64() < 0.03 {
				b.add(t, domain.ActionTakedown, attacker, map[string]any{"targetPlayerId": defender})
				continue
			}
		}
		strike := pick(rng, []string{"JAB", "CROSS", "HOOK", "KICK"})
		b.add(t, domain.ActionStrikeAttempt, attacker, map[string]any{"targetPlayerId": defender, "strikeType": strike})
		if rng.Float64() >= accuracy[attacker] {
			combo = false
			continue
		}
		b.add(t, domain.ActionStrikeLanded, attacker, map[string]any{"targetPlayerId": defender, "strikeType": strike})
		landed[attacker]++
		recent = append(recent, struct {
			at      float64
			fighter string
		}{t, attacker})
		streak := 0
		for _, r := range recent {
			if r.fighter == attacker && t-r.at <= 5 {
				streak++
			}
		}
		combo = rng.Float64() < 0.55
		if rng.Float64() < 0.001+0.005*float64(streak*streak) {
			b.add(t+0.5, domain.ActionKnockdown, attacker, map[string]any{"targetPlayerId": defender})
			combo = false
			if rng.Float64() < 0.4 {
				t += 2
				b.add(t, domain.ActionRefereeStoppage, attacker, map[string]any{"victimId": defender})
				winner, reason = attacker, "KO_TKO"
				break
			}
			t += 1
		}
	}
	if winner == "" && landed[fighters[0]] != landed[fighters[1]] {
		winner = fighters[0]
		if landed[fighters[1]] > landed[fighters[0]] {
			winner = fighters[1]
		}
	}
	finish := map[string]any{"reason": reason}
	if winner != "" {
		finish["winnerPlayerId"] = winner
	} else {
		finish["reason"] = "DRAW"
	}
	b.add(t+1, domain.ActionMatchFinished, domain.SystemPlayerID, finish)
	return b.done()
}

// randomFootball: possession chains of passes; failed passes and interceptions switch
// possession, fouls interrupt the chain, and long chains have a better chance to score.
func randomFootball(id string, rng *rand.Rand, base time.Time) Match {
	teams := sample(rng, teamPool, 2)
	squads := map[string][]string{}
	for _, team := range teams {
		for number := 1; number <= 6; number++ {
			squads[team] = append(squads[team], fmt.Sprintf("%s_%02d", team, number*2-1))
		}
	}
	skill := map[string]float64{teams[0]: 0.72 + rng.Float64()*0.15, teams[1]: 0.72 + rng.Float64()*0.15}
	b := newBuilder(id, domain.GameFootball, fmt.Sprintf("random football %s vs %s", teams[0], teams[1]), base)
	b.add(0, domain.ActionMatchStarted, domain.SystemPlayerID, map[string]any{"mode": "90min"})
	goals := map[string]int{}
	attack := teams[rng.IntN(2)]
	rival := func(team string) string {
		if team == teams[0] {
			return teams[1]
		}
		return teams[0]
	}
	holder := pick(rng, squads[attack])
	chain, t := 0, 0.0
	for t < 5400 {
		// Passes inside a possession are quick; a new possession starts after a longer gap.
		if chain == 0 {
			t += 5 + rng.Float64()*15
		} else {
			t += 2 + rng.Float64()*5
		}
		if rng.Float64() < 0.04 {
			fouler := pick(rng, squads[rival(attack)])
			b.add(t, domain.ActionFoul, fouler, map[string]any{"targetPlayerId": holder, "teamId": rival(attack)})
			chain = 0
			continue
		}
		receiver := pick(rng, squads[attack])
		for receiver == holder {
			receiver = pick(rng, squads[attack])
		}
		completed := rng.Float64() < skill[attack]
		b.add(t, domain.ActionPass, holder, map[string]any{"receiverPlayerId": receiver, "completed": completed, "teamId": attack, "distanceMeters": 5 + rng.IntN(35)})
		if !completed {
			defender := pick(rng, squads[rival(attack)])
			if rng.Float64() < 0.5 {
				b.add(t+1, domain.ActionInterception, defender, map[string]any{"targetPlayerId": holder, "teamId": rival(attack)})
			}
			attack, holder, chain = rival(attack), defender, 0
			continue
		}
		holder = receiver
		chain++
		if rng.Float64() < 0.0005+0.001*float64(min(chain, 10)) {
			t += 2
			b.add(t, domain.ActionGoal, holder, map[string]any{"teamId": attack, "minute": int(t / 60)})
			goals[attack]++
			attack = rival(attack)
			holder, chain = pick(rng, squads[attack]), 0
		}
	}
	finish := map[string]any{"reason": "FULL_TIME"}
	if goals[teams[0]] != goals[teams[1]] {
		winner := teams[0]
		if goals[teams[1]] > goals[teams[0]] {
			winner = teams[1]
		}
		finish["winnerTeamId"] = winner
	}
	b.add(t+5, domain.ActionMatchFinished, domain.SystemPlayerID, finish)
	return b.done()
}

// randomRace: positions are computed from cumulative times, so crashes and pit stops
// cost places that fast riders may recover later.
func randomRace(id string, rng *rand.Rand, base time.Time) Match {
	riders := sample(rng, riderPool, 6)
	laps := 6 + rng.IntN(3)
	b := newBuilder(id, domain.GameMotorcycleRacing, fmt.Sprintf("random race, %d laps", laps), base)
	b.add(0, domain.ActionMatchStarted, domain.SystemPlayerID, map[string]any{"track": "Circuito La Guácima", "totalLaps": laps})
	pace := map[string]float64{}
	noise := map[string]float64{}
	total := map[string]float64{}
	for _, rider := range riders {
		pace[rider] = 88 + rng.Float64()*6
		noise[rider] = 0.2 + rng.Float64()*1.3 // seconds of lap-to-lap variation
	}
	position := func(lap int, rider string, times map[string]float64) int {
		rank := 1
		for _, other := range riders {
			if other != rider && times[other] < times[rider] {
				rank++
			}
		}
		return rank
	}
	lastPosition := map[string]int{}
	for i, rider := range riders {
		lastPosition[rider] = i + 1
	}
	for lap := 1; lap <= laps; lap++ {
		lapTimes := map[string]float64{}
		for _, rider := range riders {
			lapTime := pace[rider] + rng.NormFloat64()*noise[rider]
			roll := rng.Float64()
			switch {
			case lap > 1 && roll < 0.05:
				at := total[rider] + lapTime*0.4
				b.add(at, domain.ActionCrash, rider, map[string]any{"position": lastPosition[rider]})
				lost := 20 + rng.Float64()*20
				b.add(at+lost, domain.ActionRejoinRace, rider, map[string]any{"position": len(riders)})
				lapTime += lost
			case lap > 1 && lap < laps && roll < 0.10:
				at := total[rider] + lapTime*0.9
				b.add(at, domain.ActionPitStop, rider, map[string]any{"position": lastPosition[rider]})
				lapTime += 22 + rng.Float64()*6
			}
			lapTimes[rider] = lapTime
			total[rider] += lapTime
		}
		for _, rider := range riders {
			pos := position(lap, rider, total)
			lastPosition[rider] = pos
			b.add(total[rider], domain.ActionLapCompleted, rider, map[string]any{
				"lapNumber": lap, "lapTimeMs": int(lapTimes[rider] * 1000), "position": pos,
				"topSpeedKmh": 280 + rng.IntN(60),
			})
		}
	}
	winner, end := "", 0.0
	for _, rider := range riders {
		if lastPosition[rider] == 1 {
			winner = rider
		}
		if total[rider] > end {
			end = total[rider]
		}
	}
	b.add(end+1, domain.ActionMatchFinished, domain.SystemPlayerID, map[string]any{"winnerPlayerId": winner, "reason": "CHECKERED_FLAG"})
	return b.done()
}

// randomBattleRoyale: players with different skill trade damage until one survives;
// whoever lands the lethal hit gets the kill, so third parties happen naturally.
func randomBattleRoyale(id string, rng *rand.Rand, base time.Time) Match {
	players := sample(rng, battleRoyalePool(), 8+rng.IntN(5))
	b := newBuilder(id, domain.GameFreeFire, fmt.Sprintf("random battle royale, %d players", len(players)), base)
	b.add(0, domain.ActionMatchStarted, domain.SystemPlayerID, map[string]any{"map": "Bermuda"})
	health := map[string]float64{}
	skill := map[string]float64{}
	for _, player := range players {
		health[player] = 200
		skill[player] = 0.5 + rng.Float64()
	}
	alive := append([]string(nil), players...)
	weighted := func() string {
		sum := 0.0
		for _, p := range alive {
			sum += skill[p]
		}
		roll := rng.Float64() * sum
		for _, p := range alive {
			roll -= skill[p]
			if roll <= 0 {
				return p
			}
		}
		return alive[len(alive)-1]
	}
	t, phase := 0.0, 0
	for len(alive) > 1 {
		t += 1 + rng.Float64()*6
		if int(t/120) > phase {
			phase = int(t / 120)
			b.add(float64(phase*120), domain.ActionZoneShrink, domain.SystemPlayerID, map[string]any{"zonePhase": phase})
		}
		attacker := weighted()
		target := pick(rng, alive)
		for target == attacker {
			target = pick(rng, alive)
		}
		weapon := pick(rng, weaponPool)
		damage := float64(15 + rng.IntN(46))
		b.add(t, domain.ActionDamageDealt, attacker, map[string]any{"targetPlayerId": target, "damage": damage, "weapon": weapon})
		health[target] -= damage
		if health[target] <= 0 {
			b.add(t+0.2, domain.ActionPlayerEliminated, attacker, map[string]any{"victimId": target, "weapon": weapon})
			next := alive[:0]
			for _, p := range alive {
				if p != target {
					next = append(next, p)
				}
			}
			alive = next
		}
	}
	b.add(t+5, domain.ActionMatchFinished, domain.SystemPlayerID, map[string]any{"winnerPlayerId": alive[0], "reason": "LAST_SURVIVOR"})
	return b.done()
}
