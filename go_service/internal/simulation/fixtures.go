// Package simulation represents external game producers; it computes no analytics.
package simulation

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

type Match struct {
	ID       string
	Scenario string
	Events   []domain.Event
}

// Fixtures uses fixed event-time intervals, independent of HTTP transmission speed.
// There are two different scenarios per game. prefix makes repeated runs unique.
func Fixtures(prefix string) []Match {
	var matches []Match
	for _, game := range []string{domain.GameMMA, domain.GameFootball, domain.GameMotorcycleRacing, domain.GameFreeFire} {
		for variant := 0; variant < 2; variant++ {
			id := fmt.Sprintf("%s_%s_%d", prefix, game, variant+1)
			m := Match{ID: id}
			finishSecond := 60
			base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			add := func(seconds int, action, actor string, data map[string]any) {
				if data == nil {
					data = map[string]any{}
				}
				raw, _ := json.Marshal(data)
				typ := domain.TypeAction
				if action == domain.ActionMatchStarted || action == domain.ActionMatchFinished {
					typ = domain.TypeMatch
				}
				m.Events = append(m.Events, domain.Event{EventID: fmt.Sprintf("%s_evt_%03d", id, len(m.Events)), Timestamp: base.Add(time.Duration(seconds) * time.Second), GameID: game, MatchID: id, PlayerID: actor, Type: typ, Action: action, Data: raw})
			}
			p1, p2, p3 := id+"_p1", id+"_p2", id+"_p3"
			add(0, domain.ActionMatchStarted, domain.SystemPlayerID, nil)
			switch game {
			case domain.GameMMA:
				m.Scenario = "three landed strikes followed by knockdown"
				gap := 1
				if variant == 1 {
					gap = 10
					m.Scenario = "separated strikes without knockdown"
				}
				add(1, domain.ActionStrikeAttempt, p2, map[string]any{"targetPlayerId": p1})
				for i := 1; i <= 3; i++ {
					add(i*gap, domain.ActionStrikeAttempt, p1, map[string]any{"targetPlayerId": p2})
					add(i*gap, domain.ActionStrikeLanded, p1, map[string]any{"targetPlayerId": p2})
				}
				if variant == 0 {
					add(3, domain.ActionKnockdown, p1, map[string]any{"targetPlayerId": p2})
					add(4, domain.ActionRefereeStoppage, p1, map[string]any{"victimId": p2})
				}
				if variant == 0 {
					finishSecond = 5
				}
			case domain.GameFootball:
				m.Scenario = "five complete same-team passes and goal within 30 seconds"
				if variant == 1 {
					m.Scenario = "pass sequence interrupted by foul and incomplete pass"
				}
				for i := 1; i <= 5; i++ {
					actor, receiver := p1, p2
					if i%2 == 0 {
						actor, receiver = p2, p1
					}
					add(i*3, domain.ActionPass, actor, map[string]any{"receiverPlayerId": receiver, "completed": variant == 0 || i != 4, "teamId": "home"})
					if variant == 1 && i == 3 {
						add(10, domain.ActionFoul, p3, map[string]any{"targetPlayerId": p1, "teamId": "away"})
					}
				}
				add(18, domain.ActionGoal, p2, map[string]any{"teamId": "home"})
			case domain.GameMotorcycleRacing:
				m.Scenario = "recovery after crash with improving positions"
				if variant == 1 {
					m.Scenario = "pit stop with no recovery to first place"
				}
				finishSecond = 500
				add(91, domain.ActionLapCompleted, p1, map[string]any{"lapNumber": 1, "lapTimeMs": 91000, "position": 4})
				add(90, domain.ActionLapCompleted, p2, map[string]any{"lapNumber": 1, "lapTimeMs": 90000, "position": 2})
				action := domain.ActionCrash
				if variant == 1 {
					action = domain.ActionPitStop
				}
				add(100, action, p1, map[string]any{"position": 8})
				add(120, domain.ActionRejoinRace, p1, map[string]any{"position": 8})
				crossings := []int{0, 91, 216, 306, 395}
				lapTimes := []int{0, 91000, 125000, 90000, 89000}
				if variant == 1 {
					crossings = []int{0, 91, 240, 345, 450}
					lapTimes = []int{0, 91000, 149000, 105000, 105000}
				}
				for lap := 2; lap <= 4; lap++ {
					position := 9 - lap*2
					if variant == 1 {
						position = 8
					}
					add(crossings[lap], domain.ActionLapCompleted, p1, map[string]any{"lapNumber": lap, "lapTimeMs": lapTimes[lap], "position": position})
					otherPosition := 2
					if variant == 0 && lap == 3 {
						otherPosition = 4
					}
					add(90+(lap-1)*110, domain.ActionLapCompleted, p2, map[string]any{"lapNumber": lap, "lapTimeMs": 110000, "position": otherPosition})
				}
			case domain.GameFreeFire:
				m.Scenario = "third party eliminates the recently damaged victim"
				if variant == 1 {
					m.Scenario = "same attacker deals damage and eliminates the victim"
				}
				add(1, domain.ActionDamageDealt, p1, map[string]any{"targetPlayerId": p2, "damage": 75})
				killer := p3
				if variant == 1 {
					killer = p1
				}
				add(3, domain.ActionPlayerEliminated, killer, map[string]any{"victimId": p2, "weapon": "SNIPER"})
				add(8, domain.ActionDamageDealt, p3, map[string]any{"targetPlayerId": p1, "damage": 25})
				add(15, domain.ActionZoneShrink, domain.SystemPlayerID, map[string]any{"zonePhase": 2})
			}
			add(finishSecond, domain.ActionMatchFinished, domain.SystemPlayerID, nil)
			sort.SliceStable(m.Events, func(i, j int) bool { return m.Events[i].Timestamp.Before(m.Events[j].Timestamp) })
			matches = append(matches, m)
		}
	}
	return matches
}
