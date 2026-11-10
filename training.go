package pokeedit

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// EvolutionChoice is one direct evolution of a Pokémon and whether it can
// happen right now.
type EvolutionChoice struct {
	Target       int
	Name         string
	Ready        bool
	Requirement  string
	ConsumesItem bool
}

// personalForGame is the table that matches the game a Pokémon lives in.
func personalForGame(pk *Pokemon, game Game) *PersonalTable {
	switch pk.Format() {
	case 1:
		if game == Yellow {
			return personalTable("y", 1)
		}
		return personalTable("rb", 1)
	case 2:
		if game == Crystal {
			return personalTable("c", 2)
		}
		return personalTable("gs", 2)
	}
	switch game {
	case FireRed, FireRedLeafGreen:
		return personalTable("fr", 3)
	case LeafGreen:
		return personalTable("lg", 3)
	case Emerald:
		return personalTable("e", 3)
	}
	return personalTable("rs", 3)
}

// CalcStatsFor computes a Pokémon's stats using a game's personal table.
func CalcStatsFor(pk *Pokemon, game Game) [6]int {
	return pk.CalcStats(personalForGame(pk, game).Get(pk.Species()))
}

// MovePool returns every move a Pokémon may (re)learn at its current level in
// a game: level-up moves, starting moves (generation 1), its current moves and
// any moves in remembered, in ascending order.
func MovePool(pk *Pokemon, game Game, remembered []int) []int {
	set := map[int]bool{}
	for _, m := range LearnsetOf(game, pk.Species()).MoveRange(pk.Level()) {
		set[int(m)] = true
	}
	if pk.Format() == 1 {
		for _, m := range personalForGame(pk, game).Get(pk.Species()).StarterMoves {
			set[int(m)] = true
		}
	}
	for _, m := range pk.Moves() {
		set[m] = true
	}
	for _, m := range remembered {
		set[m] = true
	}
	max := MaxMove(pk.Format())
	var out []int
	for m := range set {
		if m > 0 && m <= max {
			out = append(out, m)
		}
	}
	sort.Ints(out)
	return out
}

// EvolutionChoices lists a Pokémon's direct evolutions with their current
// eligibility. now is the clock used for time-of-day evolutions.
func EvolutionChoices(pk *Pokemon, game Game, now time.Time) []EvolutionChoice {
	items := ItemNames(pk.Format())
	item := func(id int) string {
		if id >= 0 && id < len(items) {
			return items[id]
		}
		return fmt.Sprintf("evolution item %d", id)
	}
	hour := now.UTC().Hour()
	var daytime bool
	if pk.Format() == 3 {
		daytime = hour >= 12
	} else {
		daytime = hour >= 4 && hour < 18
	}
	stats := CalcStatsFor(pk, game)
	max := MaxSpecies(pk.Format())
	level := pk.Level()
	var out []EvolutionChoice
	for _, evo := range EvolutionsOf(pk.Format(), pk.Species()) {
		if evo.Species == 0 || evo.Species > max {
			continue
		}
		ready, consume := true, false
		var req string
		lv := evo.Level
		if lv < 2 {
			lv = 2
		}
		arg := evo.Argument
		if pk.Format() == 2 && (evo.Method == EvoUseItem || evo.Method == EvoTradeHeldItem) {
			name := ItemName(3, arg)
			arg = -1
			for i, n := range items {
				if strings.EqualFold(n, name) {
					arg = i
					break
				}
			}
		}
		switch evo.Method {
		case EvoLevelUp, EvoLevelUpNinjask:
			ready, req = level >= lv, fmt.Sprintf("Level %d", lv)
		case EvoLevelUpFriendship, EvoLevelUpFriendshipMorning, EvoLevelUpFriendshipNight:
			ready = level >= 2 && pk.Friendship() >= 220
			req = fmt.Sprintf("Friendship 220 (currently %d)", pk.Friendship())
			if evo.Method != EvoLevelUpFriendship {
				day := evo.Method == EvoLevelUpFriendshipMorning
				ready = ready && day == daytime
				if day {
					req += "; daytime by the server UTC game clock"
				} else {
					req += "; nighttime by the server UTC game clock"
				}
				if game == FireRed || game == LeafGreen || game == FireRedLeafGreen {
					ready, req = false, "Clock-based evolution requires Ruby, Sapphire or Emerald"
				}
			}
		case EvoUseItem:
			req = "Garden supplies " + item(arg)
		case EvoTrade:
			req = "Garden arranges a trade evolution"
		case EvoTradeHeldItem:
			ready = arg >= 0 && pk.HeldItem() == arg
			consume = true
			req = "Garden trade while holding " + item(arg) + " (consumed)"
		case EvoLevelUpATK, EvoLevelUpAeqD, EvoLevelUpDEF:
			var matches bool
			sym := "="
			switch evo.Method {
			case EvoLevelUpATK:
				matches, sym = stats[1] > stats[2], ">"
			case EvoLevelUpDEF:
				matches, sym = stats[1] < stats[2], "<"
			default:
				matches = stats[1] == stats[2]
			}
			ready = level >= lv && matches
			req = fmt.Sprintf("Level %d; Attack %s Defense", lv, sym)
		case EvoLevelUpECl5, EvoLevelUpECgeq5:
			first := (pk.PID()>>16)%10 < 5
			ready = level >= lv && first == (evo.Method == EvoLevelUpECl5)
			req = fmt.Sprintf("Level %d; branch determined by original personality", lv)
		case EvoLevelUpBeauty:
			ready = level >= 2 && pk.Format() == 3 && pk.ContestBeauty() >= evo.Argument
			req = fmt.Sprintf("Beauty %d", evo.Argument)
		case EvoLevelUpShedinja:
			ready, req = false, "Obtain Shedinja in-game when evolving Nincada; requires a spare party slot and Poké Ball"
		default:
			ready, req = false, "This special evolution must be performed in-game"
		}
		if pk.HeldItem() > 0 && strings.EqualFold(item(pk.HeldItem()), "Everstone") {
			ready, req = false, "Remove the held Everstone first"
		}
		if game == Yellow && pk.Species() == 25 {
			ready, req = false, "Yellow starter Pikachu cannot evolve here; use Red/Blue"
		}
		out = append(out, EvolutionChoice{
			Target: evo.Species, Name: SpeciesName(evo.Species), Ready: ready,
			Requirement: req, ConsumesItem: consume,
		})
	}
	return out
}

// Evolve performs a direct evolution after re-checking eligibility.
func Evolve(pk *Pokemon, game Game, target int, now time.Time) error {
	if err := require(!pk.IsEgg(), "Let this egg hatch before training."); err != nil {
		return err
	}
	var choice *EvolutionChoice
	for _, c := range EvolutionChoices(pk, game, now) {
		if c.Target == target {
			c := c
			choice = &c
			break
		}
	}
	if err := require(choice != nil, "This is not a direct evolution for this Pokémon in its game."); err != nil {
		return err
	}
	if err := require(choice.Ready, "Evolution is not ready: "+choice.Requirement); err != nil {
		return err
	}
	nickname := pk.Nickname()
	defaultName := !pk.IsNicknamed()
	pk.SetSpecies(target)
	if defaultName {
		lang := pk.Language()
		if lang == LanguageNone {
			lang = LanguageEnglish
		}
		pk.SetNickname(defaultSpeciesNickname(pk.Species(), lang, pk.Format()))
	} else {
		pk.SetNickname(nickname)
	}
	if choice.ConsumesItem {
		pk.SetHeldItem(0)
	}
	pk.RecalculateStats(personalForGame(pk, game).Get(pk.Species()))
	pk.RefreshChecksum()
	return nil
}

// Relearn replaces move slot with move, which must be in the Pokémon's move
// pool and not already known. Only the replaced slot's PP is reset.
func Relearn(pk *Pokemon, game Game, slot, move int, remembered []int) error {
	if err := require(!pk.IsEgg(), "Let this egg hatch before training."); err != nil {
		return err
	}
	if err := require(slot >= 0 && slot <= 3, "Choose one of the four move slots."); err != nil {
		return err
	}
	pool := MovePool(pk, game, remembered)
	if err := require(containsInt(pool, move) && move > 0 && move <= MaxMove(pk.Format()),
		"This move cannot be relearned at the current level in this game."); err != nil {
		return err
	}
	cur := pk.Moves()
	if err := require(!containsInt(cur[:], move), "This Pokémon already knows that move."); err != nil {
		return err
	}
	pk.SetMove(slot, move)
	pk.SetMovePPUps(slot, 0)
	pk.SetMovePP(slot, BasePP(pk.Format(), move))
	pk.RefreshChecksum()
	return nil
}

// Profile is a display-oriented summary of a Pokémon.
type Profile struct {
	SpeciesName string
	Egg         bool
	Level       int
	Gender      string
	Nature      string // empty before generation 3
	Shiny       bool
	HeldItem    string // empty when none
	Trainer     string
	TrainerID   int
	Stats       [6]int
	IVs         [6]int
	EVs         [6]int
	Moves       []ProfileMove
}

// ProfileMove is one known move.
type ProfileMove struct {
	Slot int
	Move int
	Name string
	PP   int
}

// ProfileOf summarizes a Pokémon as seen in a game.
func ProfileOf(pk *Pokemon, game Game) Profile {
	if !pk.PartyStatsPresent() {
		pk.ResetPartyStats()
	}
	p := Profile{
		SpeciesName: SpeciesName(pk.Species()), Egg: pk.IsEgg(), Level: pk.Level(),
		Shiny: pk.IsShiny(), Trainer: pk.OTName(), TrainerID: pk.TID(),
		Stats: CalcStatsFor(pk, game),
	}
	switch pk.Gender() {
	case 0:
		p.Gender = "male"
	case 1:
		p.Gender = "female"
	default:
		p.Gender = "genderless"
	}
	if pk.Format() == 3 {
		p.Nature = NatureName(pk.Nature())
	}
	if pk.Format() >= 2 && pk.HeldItem() > 0 {
		p.HeldItem = ItemName(pk.Format(), pk.HeldItem())
	}
	for i := 0; i < 6; i++ {
		p.IVs[i], p.EVs[i] = pk.IV(i), pk.EV(i)
	}
	for slot, m := range pk.Moves() {
		if m != 0 {
			p.Moves = append(p.Moves, ProfileMove{Slot: slot, Move: m, Name: MoveName(m), PP: pk.MovePP(slot)})
		}
	}
	return p
}
