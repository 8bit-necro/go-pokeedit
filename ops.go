package pokeedit

import "math/rand/v2"

// Slot addresses a party slot (Box < 0) or a box slot.
type Slot struct {
	Box  int // -1 for the party
	Slot int
}

// PartySlot addresses a party slot.
func PartySlot(i int) Slot { return Slot{Box: -1, Slot: i} }

// BoxSlot addresses a box slot.
func BoxSlot(box, slot int) Slot { return Slot{Box: box, Slot: slot} }

// GetPokemon reads the Pokémon at a slot, failing for empty slots.
func GetPokemon(sav Save, at Slot) (*Pokemon, error) {
	var (
		pk  *Pokemon
		err error
	)
	if at.Box < 0 {
		pk, err = sav.Party(at.Slot)
	} else {
		if at.Box >= sav.BoxCount() {
			return nil, fail("Invalid box.")
		}
		pk, err = sav.Box(at.Box, at.Slot)
	}
	if err != nil {
		return nil, err
	}
	if pk == nil || pk.Species() == 0 {
		return nil, fail("Select an occupied Pokémon slot.")
	}
	return pk, nil
}

// SetPokemon writes a Pokémon (refreshing its checksum) over an occupied slot.
func SetPokemon(sav Save, at Slot, pk *Pokemon) error {
	pk.RefreshChecksum()
	if at.Box < 0 {
		return sav.SetParty(at.Slot, pk)
	}
	return sav.SetBox(at.Box, at.Slot, pk)
}

// Recalculate recomputes a Pokémon's stats for this save's game, keeping its
// HP (clamped), status and PP.
func Recalculate(sav Save, pk *Pokemon) {
	pp := [4]int{pk.MovePP(0), pk.MovePP(1), pk.MovePP(2), pk.MovePP(3)}
	pk.RecalculateStats(sav.Personal().Get(pk.Species()))
	for i, v := range pp {
		pk.SetMovePP(i, v)
	}
}

// PlaceNew stores a Pokémon in the first free party slot (box < 0) or box
// slot, recalculating its stats and registering it in the Pokédex.
func PlaceNew(sav Save, pk *Pokemon, box int) error {
	var at Slot
	if box < 0 {
		if err := require(sav.PartyCount() < 6, "The party is full."); err != nil {
			return err
		}
		at = PartySlot(sav.PartyCount())
	} else {
		if err := require(box < sav.BoxCount(), "Invalid box."); err != nil {
			return err
		}
		slot := sav.FirstFreeBoxSlot(box)
		if err := require(slot >= 0, "This box is full or locked."); err != nil {
			return err
		}
		at = BoxSlot(box, slot)
	}
	Recalculate(sav, pk)
	if err := SetPokemon(sav, at, pk); err != nil {
		return err
	}
	if !pk.IsEgg() {
		sav.SetSeen(pk.Species(), true)
		sav.SetCaught(pk.Species(), true)
	}
	return nil
}

func learnsetGame(sav Save) Game { return sav.Game() }

// AddPokemon creates a level-appropriate Pokémon owned by the player and
// stores it in the party (box < 0) or a box.
func AddPokemon(sav Save, species, level, box int) (*Pokemon, error) {
	if err := require(species > 0 && species <= sav.MaxSpecies() && sav.Personal().InGame(species),
		"Species is not supported by this game."); err != nil {
		return nil, err
	}
	if err := require(level >= 2 && level <= 100, "Level must be 2–100."); err != nil {
		return nil, err
	}
	pk := sav.BlankPokemon()
	pk.SetSpecies(species)
	pk.SetLevel(level)
	pk.SetOTName(sav.OT())
	pk.SetTID(sav.TID())
	pk.SetSID(sav.SID())
	lang := sav.Language()
	if lang == LanguageNone {
		lang = LanguageEnglish
	}
	pk.SetLanguage(lang)
	pk.SetNickname(defaultSpeciesNickname(species, lang, sav.Generation()))
	if pk.Format() == 3 {
		pk.SetOriginGame(sav.Game().gen3Version())
		pk.SetPID(rand.Uint32())
		for i := 0; i < 6; i++ {
			pk.SetIV(i, 15)
		}
		pk.SetFriendship(int(sav.Personal().Get(species).BaseFriendship))
		pk.SetBall(4)
		pk.SetMetLevel(level)
	} else {
		pk.SetDV16(0x8888)
		if pk.Format() == 2 {
			pk.SetFriendship(int(sav.Personal().Get(species).BaseFriendship))
		}
	}
	var moves [4]uint16
	ctr := 0
	if sav.Generation() == 1 {
		for i, m := range sav.Personal().Get(species).StarterMoves {
			moves[i] = uint16(m)
			if m != 0 {
				ctr++
			}
		}
	}
	LearnsetOf(learnsetGame(sav), species).EncounterMoves(level, &moves, ctr)
	for i, m := range moves {
		pk.SetMove(i, int(m))
	}
	pk.HealPP()
	pk.RefreshChecksum()
	pk.ResetPartyStats()
	if err := PlaceNew(sav, pk, box); err != nil {
		return nil, err
	}
	return pk, nil
}

// HealPokemon restores HP, status and PP of the Pokémon at a slot.
func HealPokemon(sav Save, at Slot) error {
	pk, err := GetPokemon(sav, at)
	if err != nil {
		return err
	}
	Recalculate(sav, pk)
	pk.SetHP(pk.MaxHP())
	pk.SetStatus(0)
	pk.HealPP()
	return SetPokemon(sav, at, pk)
}

// partyAllEggsExcept reports whether every party member besides slot is an egg.
func partyAllEggsExcept(sav Save, slot int) bool {
	for i := 0; i < sav.PartyCount(); i++ {
		if i == slot {
			continue
		}
		pk, err := sav.Party(i)
		if err == nil && !pk.IsEgg() {
			return false
		}
	}
	return true
}

// ValidateEntity checks a Pokémon decoded from an external record.
func ValidateEntity(pk *Pokemon) error {
	if !(pk.Species() > 0 && pk.Species() <= 386 && pk.Valid() && pk.ChecksumValid()) {
		return fail("Only valid GB/GBC/GBA Pokémon and eggs are supported.")
	}
	if err := require(pk.Species() <= MaxSpecies(pk.Format()), "Species is not supported by this Pokémon format."); err != nil {
		return err
	}
	return require(pk.EXP() <= ExpForLevel(100, pk.Growth()), "Experience exceeds level 100.")
}

// PrepareExport ensures a Pokémon carries party stats before it leaves a save.
func PrepareExport(pk *Pokemon) {
	if pk.PartyStatsPresent() {
		return
	}
	hp := pk.HP()
	pk.ResetPartyStats()
	if pk.Format() == 1 && hp < pk.MaxHP() {
		pk.SetHP(hp)
	}
}

// Deposit removes the Pokémon at a slot from the save and returns it, ready to
// be stored elsewhere (for example in the garden).
func Deposit(sav Save, at Slot) (*Pokemon, error) {
	pk, err := GetPokemon(sav, at)
	if err != nil {
		return nil, err
	}
	if !(pk.Species() != 0 && pk.Valid() && pk.ChecksumValid()) {
		return nil, fail("Choose a valid Pokémon or egg.")
	}
	if pk.Format() == 3 && IsMail(pk.HeldItem()) {
		return nil, fail("Remove attached mail before sending this Pokémon to the garden.")
	}
	if at.Box < 0 {
		if !(sav.PartyCount() > 1 && !partyAllEggsExcept(sav, at.Slot)) {
			return nil, fail("Keep at least one non-egg Pokémon in your party.")
		}
		if err := sav.DeleteParty(at.Slot); err != nil {
			return nil, err
		}
	} else if err := sav.DeleteBox(at.Box, at.Slot); err != nil {
		return nil, err
	}
	PrepareExport(pk)
	return pk, nil
}

// Withdraw converts a Pokémon for this save's game and stores it in the first
// free party slot (box < 0) or box slot.
func Withdraw(sav Save, pk *Pokemon, box int) error {
	if err := ValidateEntity(pk); err != nil {
		return err
	}
	conv, err := pk.ConvertFor(sav.Generation(), sav.Language() == LanguageJapanese && sav.Generation() <= 2)
	if err != nil {
		return err
	}
	if err := require(sav.Personal().InGame(conv.Species()), "This species is not supported by the destination game."); err != nil {
		return err
	}
	if err := require(conv.Format() == sav.Generation(), "Pokémon format is incompatible with this game."); err != nil {
		return err
	}
	return PlaceNew(sav, conv, box)
}

// AllGames lists the game codes compatibility is evaluated against.
var compatibilityGames = []Game{Red, Blue, Green, RedBlue, Yellow, Gold, Silver, GoldSilver, Crystal,
	Ruby, Sapphire, RubySapphire, Emerald, FireRed, LeafGreen, FireRedLeafGreen}

// CompatibleGames lists the games a Pokémon could be moved into.
func CompatibleGames(pk *Pokemon) []Game {
	var out []Game
	for _, g := range compatibilityGames {
		if _, err := pk.ConvertFor(g.Generation(), pk.format <= 2 && pk.Japanese()); err == nil {
			out = append(out, g)
		}
	}
	return out
}

// Hatch turns a ready egg (friendship 0) into a hatchling. Generation 3 eggs
// carry a Japanese language flag; the hatchling gets lang (English when zero).
func Hatch(pk *Pokemon, origin Game, lang Language) error {
	if err := require(pk.IsEgg() && pk.Friendship() == 0, "This egg is not ready to hatch."); err != nil {
		return err
	}
	if lang == LanguageNone {
		lang = LanguageEnglish
	}
	pk.SetEgg(false)
	pk.SetFriendship(int(defaultPersonal(pk.Format()).Get(pk.Species()).BaseFriendship))
	if pk.Format() == 3 {
		pk.SetLanguage(lang)
		pk.SetNickname(defaultSpeciesNickname(pk.Species(), lang, 3))
		pk.SetMetLevel(0)
		if origin == FireRed || origin == LeafGreen || origin == FireRedLeafGreen {
			pk.SetMetLocation(88)
		} else {
			pk.SetMetLocation(1)
		}
	} else {
		pk.SetNotNicknamed(pk.guessedLanguage())
		pk.SetMetLevel(1)
		pk.SetMetLocation(1)
	}
	pk.ResetPartyStats()
	pk.HealPP()
	pk.RefreshChecksum()
	return nil
}

// ---------------------------------------------------------------------------
// Breeding

const (
	eggGroupDitto        = 13
	eggGroupUndiscover   = 15
	speciesDitto         = 132
	eggPartialIVStats    = 3
	eggLevel             = 5
	eggHatchFriendship   = 120
	hatchLocationCrystal = 16
	hatchLocationRSE     = 32
	hatchLocationFRLG    = 146
)

var cannotHatch = map[int]bool{
	132: true, 144: true, 145: true, 146: true, 150: true, 151: true,
	201: true, 243: true, 244: true, 245: true, 249: true, 250: true, 251: true,
	377: true, 378: true, 379: true, 380: true, 381: true, 382: true, 383: true, 384: true, 385: true, 386: true,
}

func eggGame(g Game) Game {
	switch g {
	case GoldSilver:
		return Gold
	case RubySapphire:
		return Ruby
	case FireRedLeafGreen:
		return FireRed
	}
	return g
}

func eggGroups(pk *Pokemon) []int {
	if pk.Format() < 2 {
		return nil
	}
	p := pk.personal()
	return []int{p.EggGroup1, p.EggGroup2}
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// EggResult is the outcome of a successful breeding.
type EggResult struct {
	Egg       *Pokemon
	Mother    string
	Inherited []string
}

// Breed produces an egg from two non-egg parents of the same generation. The
// mother's game decides the egg's origin.
func Breed(a, b *Pokemon, motherGame func(mother *Pokemon) Game, rng *rand.Rand) (*EggResult, error) {
	if rng == nil {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	if err := require(!a.IsEgg() && !b.IsEgg(), "Eggs cannot raise an egg."); err != nil {
		return nil, err
	}
	if err := require(a.Format() >= 2 && a.Format() == b.Format(),
		"Only Generation II or III Pokémon from the same game generation can raise an egg together."); err != nil {
		return nil, err
	}
	ga, gb := eggGroups(a), eggGroups(b)
	if err := require(!containsInt(ga, eggGroupUndiscover) && !containsInt(gb, eggGroupUndiscover),
		"One of these Pokémon cannot have eggs."); err != nil {
		return nil, err
	}
	var mother *Pokemon
	if a.Species() == speciesDitto || b.Species() == speciesDitto {
		if err := require(a.Species() != b.Species(), "Two Dittos cannot make an egg."); err != nil {
			return nil, err
		}
		mother = a
		if a.Species() == speciesDitto {
			mother = b
		}
	} else {
		shared := false
		for _, g := range ga {
			if g != 0 && g != eggGroupDitto && containsInt(gb, g) {
				shared = true
			}
		}
		if err := require(shared, "These two do not share an egg group."); err != nil {
			return nil, err
		}
		g1, g2 := a.Gender(), b.Gender()
		if err := require((g1 == 0 && g2 == 1) || (g1 == 1 && g2 == 0),
			"An egg needs one male and one female parent (or a Ditto)."); err != nil {
			return nil, err
		}
		mother = a
		if a.Gender() != 1 {
			mother = b
		}
	}
	game := motherGame(mother)
	species := baseSpecies(mother.Format(), mother.Species())
	if err := require(!cannotHatch[species], "This family cannot hatch from an egg."); err != nil {
		return nil, err
	}
	egg := newEgg(mother, species, eggGame(game), rng)
	var inherited []string
	if egg.Format() == 3 {
		order := rng.Perm(6)[:eggPartialIVStats]
		for _, stat := range order {
			src := a
			if rng.IntN(2) != 0 {
				src = b
			}
			egg.SetIV(stat, src.IV(stat))
		}
		inherited = append(inherited, "stats")
	}
	learnable := map[int]bool{}
	for _, m := range LearnsetOf(eggGame(game), species).MoveRange(100) {
		learnable[int(m)] = true
	}
	var shared []int
	seen := map[int]bool{}
	bm := b.Moves()
	for _, m := range a.Moves() {
		if m > 0 && containsInt(bm[:], m) && learnable[m] && !seen[m] {
			seen[m] = true
			shared = append(shared, m)
		}
	}
	var moves []int
	for _, m := range egg.Moves() {
		if m > 0 {
			moves = append(moves, m)
		}
	}
	added := 0
	for _, m := range shared {
		if containsInt(moves, m) {
			continue
		}
		if added == 2 {
			break
		}
		added++
		if len(moves) < 4 {
			moves = append(moves, m)
		} else {
			moves[3] = m
		}
		inherited = append(inherited, MoveName(m))
	}
	for i := 0; i < 4; i++ {
		m := 0
		if i < len(moves) {
			m = moves[i]
		}
		egg.SetMove(i, m)
		egg.SetMovePP(i, egg.MaxMovePP(m, egg.MovePPUps(i)))
	}
	egg.HealPP()
	egg.SetEgg(true)
	egg.SetFriendship(int(egg.personal().HatchCycles))
	egg.ResetPartyStats()
	egg.RefreshChecksum()
	return &EggResult{Egg: egg, Mother: mother.Nickname(), Inherited: inherited}, nil
}

func newEgg(mother *Pokemon, species int, game Game, rng *rand.Rand) *Pokemon {
	lang := mother.Language()
	if lang == LanguageNone {
		lang = LanguageEnglish
	}
	if mother.Format() == 2 {
		pk := NewPokemon(2, mother.Japanese())
		pk.SetSpecies(species)
		pk.SetLevel(eggLevel)
		pk.SetTID(mother.TID())
		pk.SetOTName(mother.OTName())
		pk.SetFriendship(eggHatchFriendship)
		pk.SetDV16(rng.IntN(65536))
		pk.SetNotNicknamed(lang)
		if game == Crystal {
			pk.SetMetLocation(hatchLocationCrystal)
			pk.SetMetLevel(1)
			tod := 1 + rng.IntN(3)
			putBE16(pk.data[0x1D:], be16(pk.data[0x1D:])&0x3FFF|tod<<14)
			pk.SetOTGender(mother.OTGender() & 1)
		}
		setEggMoves(pk, species, game)
		return pk
	}
	pk := NewPokemon(3, false)
	pk.SetSpecies(species)
	pk.SetLevel(eggLevel)
	pk.SetOriginGame(game.gen3Version())
	pk.SetBall(4)
	pk.SetTID(mother.TID())
	pk.SetSID(mother.SID())
	pk.SetOTGender(mother.OTGender())
	pk.SetLanguage(lang)
	pk.SetNickname(defaultSpeciesNickname(species, lang, 3))
	pk.SetFriendship(eggHatchFriendship)
	pk.SetMetLevel(0)
	if game == FireRed || game == LeafGreen {
		pk.SetMetLocation(hatchLocationFRLG)
	} else {
		pk.SetMetLocation(hatchLocationRSE)
	}
	for i := range pk.data[0x14:0x1B] {
		pk.data[0x14+i] = 0xFF
	}
	pk.SetOTName(mother.OTName())
	setEggMoves(pk, species, game)
	// Three random IVs are perfect; the caller then inherits three from parents.
	for i := 0; i < 6; i++ {
		pk.SetIV(i, rng.IntN(32))
	}
	for _, stat := range rng.Perm(6)[:3] {
		pk.SetIV(stat, 31)
	}
	pi := PersonalFor(Emerald).Get(species)
	for {
		pid := rng.Uint32()
		if game == Emerald {
			if pid == 0 {
				continue
			}
		} else if pid&1 == 0 {
			continue
		}
		pk.SetPID(pid)
		break
	}
	pk.SetAbilityBit(pk.PID()%2 == 1 && pi.HasSecondAbility() &&
		species != 326 && species != 329 && species != 330)
	return pk
}

func setEggMoves(pk *Pokemon, species int, game Game) {
	lg := game
	if pk.Format() == 3 {
		switch game {
		case Ruby, Sapphire, Emerald, RubySapphire:
			lg = RubySapphire
		case FireRed, LeafGreen, FireRedLeafGreen:
			lg = FireRed
		}
	}
	initial := LearnsetOf(lg, species).BaseEggMoves(eggLevel)
	for i := 0; i < 4; i++ {
		m := 0
		if i < len(initial) {
			m = int(initial[i])
		}
		pk.SetMove(i, m)
		pk.SetMovePPUps(i, 0)
		pk.SetMovePP(i, pk.MaxMovePP(m, 0))
	}
}
