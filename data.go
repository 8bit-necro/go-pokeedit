package pokeedit

import (
	"encoding/binary"
	"sync"
)

// ---------------------------------------------------------------------------
// Species id conversion between national dex numbers and internal ids.

func internalSpecies1(species int) byte {
	if species < 0 || species >= len(species1NationalToInternal) {
		return 0
	}
	return species1NationalToInternal[species]
}

func nationalSpecies1(raw byte) int { return int(species1InternalToNational[raw]) }

// Gen1InternalToNational converts a Generation 1 internal species index (the
// first byte of a Red/Blue Pokémon record) to its national Pokédex number
// (0 for glitch/unused ids).
func Gen1InternalToNational(id int) int {
	if id < 0 || id > 255 {
		return 0
	}
	return nationalSpecies1(byte(id))
}

const (
	firstUnalignedNational3 = 252
	firstUnalignedInternal3 = 277
)

func internalSpecies3(species int) int {
	shift := species - firstUnalignedNational3
	if shift < 0 || shift >= len(species3NationalToInternalDelta) {
		return species
	}
	return species + int(species3NationalToInternalDelta[shift])
}

func nationalSpecies3(raw int) int {
	if raw < firstUnalignedNational3 {
		return raw
	}
	shift := raw - firstUnalignedInternal3
	if shift < 0 || shift >= len(species3InternalToNationalDelta) {
		return 0
	}
	return raw + int(species3InternalToNationalDelta[shift])
}

// ---------------------------------------------------------------------------
// Experience

// Growth rates as stored in the personal tables.
const (
	GrowthMediumFast  = 0
	GrowthErratic     = 1
	GrowthFluctuating = 2
	GrowthMediumSlow  = 3
	GrowthFast        = 4
	GrowthSlow        = 5
)

// GrowthName returns the pokegarden-style label for a growth rate.
func GrowthName(growth int) string {
	names := [...]string{"MEDIUM_FAST", "ERRATIC", "FLUCTUATING", "MEDIUM_SLOW", "FAST", "SLOW"}
	if growth < 0 || growth >= len(names) {
		return "MEDIUM_FAST"
	}
	return names[growth]
}

// ExpForLevel returns the minimum experience of a level (1–100).
func ExpForLevel(level, growth int) uint32 {
	if level <= 1 {
		return 0
	}
	if level > 100 {
		level = 100
	}
	if growth < 0 || growth > 5 {
		growth = 0
	}
	return growthTables[growth][level-1]
}

// LevelForExp returns the level (1–100) of an experience total.
func LevelForExp(exp uint32, growth int) int {
	if growth < 0 || growth > 5 {
		growth = 0
	}
	t := &growthTables[growth]
	if exp >= t[99] {
		return 100
	}
	l := 1
	for exp >= t[l] {
		l++
	}
	return l
}

// ---------------------------------------------------------------------------
// Binary containers

// binLinker reads PKHeX's "BinLinker" container with 16-bit offsets.
type binLinker16 []byte

func (b binLinker16) length() int { return int(binary.LittleEndian.Uint16(b[2:])) }
func (b binLinker16) entry(i int) []byte {
	o := 4 + i*2
	start := int(binary.LittleEndian.Uint16(b[o:]))
	end := int(binary.LittleEndian.Uint16(b[o+2:]))
	return b[start:end]
}

func mustRead(name string) []byte {
	b, err := dataFS.ReadFile("data/" + name)
	if err != nil {
		panic(err)
	}
	return b
}

// ---------------------------------------------------------------------------
// Personal info

// Personal is a species' static data.
type Personal struct {
	HP, ATK, DEF, SPE, SPA, SPD int
	Type1, Type2                byte
	CatchRate                   byte
	BaseEXP                     int
	Gender                      byte // gender ratio: 0 male-only, 254 female-only, 255 genderless
	HatchCycles                 byte
	BaseFriendship              byte
	EXPGrowth                   byte
	EggGroup1, EggGroup2        int
	Ability1, Ability2          int
	StarterMoves                [4]byte // generation 1 only
}

// Gender ratio magic values.
const (
	genderMale       = 0
	genderFemaleOnly = 254
	genderless       = 255
)

// HasSecondAbility reports whether a generation 3 species has two distinct abilities.
func (p *Personal) HasSecondAbility() bool { return p.Ability1 != p.Ability2 }

// PersonalTable holds the entries of one game's personal table, indexed by
// national dex number.
type PersonalTable struct {
	gen     int
	entries []Personal
}

// Get returns the entry of a species (the empty entry 0 when out of range).
func (t *PersonalTable) Get(species int) *Personal {
	if species < 0 || species >= len(t.entries) {
		return &t.entries[0]
	}
	return &t.entries[species]
}

// MaxSpecies returns the highest species of the table's generation.
func (t *PersonalTable) MaxSpecies() int { return MaxSpecies(t.gen) }

// InGame reports whether the species exists in the table's game.
func (t *PersonalTable) InGame(species int) bool {
	return species >= 0 && species <= t.MaxSpecies()
}

func parsePersonal(gen int, raw []byte) *PersonalTable {
	size := 0x1C
	if gen == 2 {
		size = 0x20
	}
	t := &PersonalTable{gen: gen, entries: make([]Personal, len(raw)/size)}
	for i := range t.entries {
		d := raw[i*size : (i+1)*size]
		p := &t.entries[i]
		switch gen {
		case 1:
			p.Gender = d[0]
			p.HP, p.ATK, p.DEF, p.SPE = int(d[1]), int(d[2]), int(d[3]), int(d[4])
			p.SPA = int(d[5])
			p.SPD = p.SPA
			p.Type1, p.Type2, p.CatchRate = d[6], d[7], d[8]
			p.BaseEXP = int(d[9])
			copy(p.StarterMoves[:], d[0x0F:0x13])
			p.EXPGrowth = d[0x13]
		case 2:
			p.HP, p.ATK, p.DEF, p.SPE, p.SPA, p.SPD = int(d[1]), int(d[2]), int(d[3]), int(d[4]), int(d[5]), int(d[6])
			p.Type1, p.Type2, p.CatchRate = d[7], d[8], d[9]
			p.BaseEXP = int(d[0xA])
			p.Gender = d[0xD]
			p.HatchCycles = d[0xF]
			p.EXPGrowth = d[0x16]
			p.EggGroup1, p.EggGroup2 = int(d[0x17]&0xF), int(d[0x17]>>4)
			p.BaseFriendship = 70
		default:
			p.HP, p.ATK, p.DEF, p.SPE, p.SPA, p.SPD = int(d[0]), int(d[1]), int(d[2]), int(d[3]), int(d[4]), int(d[5])
			p.Type1, p.Type2, p.CatchRate = d[6], d[7], d[8]
			p.BaseEXP = int(d[9])
			p.Gender = d[0x10]
			p.HatchCycles = d[0x11]
			p.BaseFriendship = d[0x12]
			p.EXPGrowth = d[0x13]
			p.EggGroup1, p.EggGroup2 = int(d[0x14]), int(d[0x15])
			p.Ability1, p.Ability2 = int(d[0x16]), int(d[0x17])
		}
	}
	return t
}

var (
	personalMu    sync.Mutex
	personalCache = map[string]*PersonalTable{}
)

func personalTable(code string, gen int) *PersonalTable {
	personalMu.Lock()
	defer personalMu.Unlock()
	if t, ok := personalCache[code]; ok {
		return t
	}
	t := parsePersonal(gen, mustRead("personal_"+code))
	personalCache[code] = t
	return t
}

// tableCode maps a game to the personal/learnset resource code.
func tableCode(g Game) string {
	switch g {
	case Yellow:
		return "y"
	case Gold, Silver, GoldSilver:
		return "gs"
	case Crystal:
		return "c"
	case Ruby, Sapphire, RubySapphire:
		return "rs"
	case Emerald:
		return "e"
	case FireRed, FireRedLeafGreen:
		return "fr"
	case LeafGreen:
		return "lg"
	}
	return "rb"
}

// PersonalFor returns the personal table of a game.
func PersonalFor(g Game) *PersonalTable { return personalTable(tableCode(g), g.Generation()) }

// defaultPersonal is the table PKHeX uses for a Pokémon without save context.
func defaultPersonal(format int) *PersonalTable {
	switch format {
	case 1:
		return personalTable("y", 1)
	case 2:
		return personalTable("c", 2)
	}
	return personalTable("rs", 3)
}

// ---------------------------------------------------------------------------
// Learnsets

// Learnset is a species' level-up moves, ordered by level. Level 0 entries are
// evolution moves that only appear in the Move Relearner.
type Learnset struct {
	Moves  []uint16
	Levels []uint8
}

// MoveRange returns the moves learned at or below maxLevel (every move for 100).
func (l *Learnset) MoveRange(maxLevel int) []uint16 {
	if maxLevel >= 100 {
		return l.Moves
	}
	end := -1
	for i := len(l.Levels) - 1; i >= 0; i-- {
		if int(l.Levels[i]) <= maxLevel {
			end = i
			break
		}
	}
	if end < 0 {
		return nil
	}
	return l.Moves[:end+1]
}

// EncounterMoves fills moves the way a wild encounter of the given level would
// know them, rotating out older moves once four are known. ctr is the number
// of moves already present.
func (l *Learnset) EncounterMoves(level int, moves *[4]uint16, ctr int) {
	for i := range l.Moves {
		req := int(l.Levels[i])
		if req < 1 {
			continue
		}
		if req > level {
			break
		}
		move := l.Moves[i]
		known := false
		for _, m := range moves {
			if m == move {
				known = true
				break
			}
		}
		if !known {
			moves[ctr&3] = move
			ctr++
		}
	}
	if ctr <= 4 {
		return
	}
	for r := ctr & 3; r > 0; r-- {
		first := moves[0]
		copy(moves[:3], moves[1:])
		moves[3] = first
	}
}

// BaseEggMoves returns the (up to four) most recent moves a hatchling knows.
func (l *Learnset) BaseEggMoves(level int) []uint16 {
	count := 0
	for _, lv := range l.Levels {
		if int(lv) > level {
			break
		}
		count++
	}
	if count == 0 {
		return nil
	}
	start := 0
	if count > 4 {
		start = count - 4
		count = 4
	}
	return l.Moves[start : start+count]
}

var (
	learnMu    sync.Mutex
	learnCache = map[string][]Learnset{}
)

// LearnsetsFor returns the level-up learnsets of a game indexed by national dex.
func LearnsetsFor(g Game) []Learnset {
	code := tableCode(g)
	learnMu.Lock()
	defer learnMu.Unlock()
	if l, ok := learnCache[code]; ok {
		return l
	}
	bl := binLinker16(mustRead("lvlmove_" + code + ".pkl"))
	out := make([]Learnset, bl.length())
	for i := 1; i < len(out); i++ {
		e := bl.entry(i)
		n := len(e) / 3
		ls := Learnset{Moves: make([]uint16, n), Levels: make([]uint8, n)}
		for j := 0; j < n; j++ {
			ls.Moves[j] = binary.LittleEndian.Uint16(e[j*2:])
		}
		copy(ls.Levels, e[n*2:n*3])
		out[i] = ls
	}
	learnCache[code] = out
	return out
}

// LearnsetOf returns the learnset of a species in a game (empty when unknown).
func LearnsetOf(g Game, species int) *Learnset {
	all := LearnsetsFor(g)
	if species < 0 || species >= len(all) {
		return &all[0]
	}
	return &all[species]
}

// ---------------------------------------------------------------------------
// Evolutions

// EvolutionType is the PKHeX evolution method id.
type EvolutionType byte

// Evolution methods that exist in generations 1–3.
const (
	EvoNone                     EvolutionType = 0
	EvoLevelUpFriendship        EvolutionType = 1
	EvoLevelUpFriendshipMorning EvolutionType = 2
	EvoLevelUpFriendshipNight   EvolutionType = 3
	EvoLevelUp                  EvolutionType = 4
	EvoTrade                    EvolutionType = 5
	EvoTradeHeldItem            EvolutionType = 6
	EvoUseItem                  EvolutionType = 8
	EvoLevelUpATK               EvolutionType = 9
	EvoLevelUpAeqD              EvolutionType = 10
	EvoLevelUpDEF               EvolutionType = 11
	EvoLevelUpECl5              EvolutionType = 12
	EvoLevelUpECgeq5            EvolutionType = 13
	EvoLevelUpNinjask           EvolutionType = 14
	EvoLevelUpShedinja          EvolutionType = 15
	EvoLevelUpBeauty            EvolutionType = 16
)

// EvolutionMethod is one way a species evolves.
type EvolutionMethod struct {
	Species  int
	Argument int
	Method   EvolutionType
	Level    int
}

var (
	evoMu    sync.Mutex
	evoCache = map[int][][]EvolutionMethod{}
)

// evolutionTree returns the forward evolution table: 1 for generation 1, 2 for
// the full generation 3 conditions applied to species 1–251, 3 for generation 3.
func evolutionTree(format int) [][]EvolutionMethod {
	key := format
	if key == 2 {
		key = 3
	}
	evoMu.Lock()
	defer evoMu.Unlock()
	if t, ok := evoCache[key]; ok {
		return t
	}
	bl := binLinker16(mustRead("evos_g" + string(rune('0'+key)) + ".pkl"))
	out := make([][]EvolutionMethod, bl.length())
	for i := range out {
		e := bl.entry(i)
		for o := 0; o+8 <= len(e); o += 8 {
			out[i] = append(out[i], EvolutionMethod{
				Method:   EvolutionType(e[o]),
				Argument: int(binary.LittleEndian.Uint16(e[o+2:])),
				Species:  int(binary.LittleEndian.Uint16(e[o+4:])),
				Level:    int(e[o+7]),
			})
		}
	}
	evoCache[key] = out
	return out
}

// EvolutionsOf lists the direct evolutions of a species in a format (1–3).
func EvolutionsOf(format, species int) []EvolutionMethod {
	t := evolutionTree(format)
	if species < 0 || species >= len(t) {
		return nil
	}
	return t[species]
}

// baseSpecies walks pre-evolutions back to the first stage.
func baseSpecies(format, species int) int {
	t := evolutionTree(format)
	max := 386
	if format == 2 {
		max = 251
	}
	parent := map[int]int{}
	for s := 1; s <= max && s < len(t); s++ {
		for _, e := range t[s] {
			if e.Species > 0 && e.Species <= max && e.Species != s {
				if _, ok := parent[e.Species]; !ok {
					parent[e.Species] = s
				}
			}
		}
	}
	cur := species
	for step := 0; step < 5; step++ {
		p, ok := parent[cur]
		if !ok {
			break
		}
		cur = p
	}
	return cur
}

// ---------------------------------------------------------------------------
// Move PP

// BasePP returns the unmodified PP of a move in a generation.
func BasePP(generation, move int) int {
	var t []byte
	switch generation {
	case 1:
		t = movePP1
	case 2:
		t = movePP2
	default:
		t = movePP3
	}
	if move < 0 || move >= len(t) {
		return 0
	}
	return int(t[move])
}
