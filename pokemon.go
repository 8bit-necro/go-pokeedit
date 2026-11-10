package pokeedit

import (
	"encoding/binary"
	"math"
)

// Sizes of the PKHeX "decrypted party" envelopes and raw structures.
const (
	size1Party  = 44
	size1Stored = 33
	size2Party  = 48
	size2Stored = 32
	size3Party  = 100
	size3Stored = 80

	strLenIntl = 11
	strLenJP   = 6

	size1UList = 1 + 2 + size1Party + 2*strLenIntl // 69
	size1JList = 1 + 2 + size1Party + 2*strLenJP   // 59
	size2UList = 1 + 2 + size2Party + 2*strLenIntl // 73
	size2JList = 1 + 2 + size2Party + 2*strLenJP   // 63
	slotEmpty  = 0xFF
	slotEgg    = 0xFD
)

// Stat indexes used by IV, EV and stat accessors (PKHeX order).
const (
	StatHP = iota
	StatAttack
	StatDefense
	StatSpeed
	StatSpecialAttack
	StatSpecialDefense
)

// Pokemon is a single Pokémon in the layout of its generation. It is a thin
// view over the raw bytes, so fields this library does not know about are
// preserved exactly when a Pokémon is read and written back.
type Pokemon struct {
	format int
	data   []byte // 44 / 48 / 100 bytes (party layout, decrypted)
	ot     []byte // generation 1/2 trainer name buffer (11 or 6 bytes)
	nick   []byte // generation 1/2 nickname buffer
	egg    bool   // generation 2 egg flag (stored in the list marker)
}

// NewPokemon returns a blank Pokémon of a format (1, 2 or 3).
func NewPokemon(format int, japanese bool) *Pokemon {
	switch format {
	case 1, 2:
		n := strLenIntl
		if japanese {
			n = strLenJP
		}
		size := size1Party
		if format == 2 {
			size = size2Party
		}
		p := &Pokemon{format: format, data: make([]byte, size), ot: make([]byte, n), nick: make([]byte, n)}
		for i := 0; i < n; i++ {
			p.ot[i], p.nick[i] = gbTerminator, gbTerminator
		}
		return p
	}
	return &Pokemon{format: 3, data: make([]byte, size3Party)}
}

// Format is the generation layout: 1, 2 or 3.
func (p *Pokemon) Format() int { return p.format }

// Japanese reports whether a generation 1/2 record uses the Japanese string size.
func (p *Pokemon) Japanese() bool { return p.format != 3 && len(p.ot) == strLenJP }

// Clone returns a deep copy.
func (p *Pokemon) Clone() *Pokemon {
	c := &Pokemon{format: p.format, data: append([]byte(nil), p.data...), egg: p.egg}
	c.ot = append([]byte(nil), p.ot...)
	c.nick = append([]byte(nil), p.nick...)
	return c
}

func be16(b []byte) int       { return int(binary.BigEndian.Uint16(b)) }
func putBE16(b []byte, v int) { binary.BigEndian.PutUint16(b, uint16(v)) }
func le16(b []byte) int       { return int(binary.LittleEndian.Uint16(b)) }
func putLE16(b []byte, v int) { binary.LittleEndian.PutUint16(b, uint16(v)) }
func le32(b []byte) uint32    { return binary.LittleEndian.Uint32(b) }
func putLE32(b []byte, v uint32) {
	binary.LittleEndian.PutUint32(b, v)
}

func (p *Pokemon) personal() *Personal {
	return defaultPersonal(p.format).Get(p.Species())
}

// ---------------------------------------------------------------------------
// Species

// Species returns the national dex number (0 for an empty slot).
func (p *Pokemon) Species() int {
	switch p.format {
	case 1:
		return nationalSpecies1(p.data[0])
	case 2:
		return int(p.data[0])
	}
	return nationalSpecies3(le16(p.data[0x20:]))
}

func (p *Pokemon) speciesInternal() int {
	switch p.format {
	case 1, 2:
		return int(p.data[0])
	}
	return le16(p.data[0x20:])
}

// SetSpecies changes the species. Types and catch rate (generation 1) follow.
func (p *Pokemon) SetSpecies(species int) {
	switch p.format {
	case 1:
		id := internalSpecies1(species)
		if p.data[0] == id {
			return
		}
		p.data[0] = id
		pi := personalTable("rb", 1).Get(species)
		p.data[5], p.data[6] = pi.Type1, pi.Type2
		if !validCatchRate(species, p.data[7]) {
			p.data[7] = pi.CatchRate
		}
	case 2:
		p.data[0] = byte(species)
	default:
		s3 := internalSpecies3(species)
		putLE16(p.data[0x20:], s3)
		if s3 != 0 {
			p.data[0x13] |= 2
		} else {
			p.data[0x13] &^= 2
		}
	}
}

func isCatchRateHeldItem(rate byte) bool {
	return catchRateIsHeldItem[rate>>3]&(1<<(rate&7)) != 0
}

func validCatchRate(species int, rate byte) bool {
	if isCatchRateHeldItem(rate) {
		return true
	}
	if species == 25 && rate == 0xA3 {
		return true
	}
	stage := 0
	if species >= 0 && species < len(evoStages1) {
		stage = int(evoStages1[species])
	}
	rb, y := personalTable("rb", 1), personalTable("y", 1)
	for s := species - stage; s <= species; s++ {
		if rate == rb.Get(s).CatchRate || rate == y.Get(s).CatchRate {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Language, names

// Language returns the (possibly guessed) language of the Pokémon.
func (p *Pokemon) Language() Language {
	if p.format == 3 {
		return Language(p.data[0x12])
	}
	if p.Japanese() {
		return LanguageJapanese
	}
	if isGermanGlyph(p.ot) {
		return LanguageGerman
	}
	nick := decodeGB(p.nick, 1, LanguageEnglish)
	return guessLanguage(p.Species(), nick, p.format)
}

// SetLanguage sets the language. For generation 1/2 this re-writes a default
// (non-nicknamed) nickname in that language.
func (p *Pokemon) SetLanguage(lang Language) {
	if p.format == 3 {
		p.data[0x12] = byte(lang)
		return
	}
	if p.Japanese() || p.IsNicknamed() {
		return
	}
	p.SetNotNicknamed(lang)
}

func (p *Pokemon) guessedLanguage() Language {
	if l := p.Language(); l > 0 {
		return l
	}
	return LanguageEnglish
}

func (p *Pokemon) nickString() string {
	switch p.format {
	case 1:
		return decodeGB(p.nick, 1, LanguageEnglish)
	case 2:
		return decodeGB(p.nick, 2, p.guessedLanguage())
	}
	return decodeG3(p.data[0x08:0x12], p.Language())
}

// Nickname returns the displayed name.
func (p *Pokemon) Nickname() string { return p.nickString() }

// MaxNicknameLength is the longest nickname the format allows.
func (p *Pokemon) MaxNicknameLength() int {
	if p.format == 3 {
		return 10
	}
	if p.Japanese() {
		return 5
	}
	return 10
}

// MaxTrainerLength is the longest trainer name the format allows.
func (p *Pokemon) MaxTrainerLength() int {
	if p.format == 3 {
		return 7
	}
	if p.Japanese() {
		return 5
	}
	return 7
}

func keepTerminatorMode(buf []byte) clearMode {
	for _, b := range buf {
		if b == 0 {
			return clearZero
		}
	}
	return clear50
}

func (p *Pokemon) gbLang() Language {
	if p.format == 1 {
		if p.Japanese() {
			return LanguageJapanese
		}
		return LanguageEnglish
	}
	return p.guessedLanguage()
}

// SetNickname changes the nickname.
func (p *Pokemon) SetNickname(name string) {
	switch p.format {
	case 3:
		if p.IsEgg() {
			name = eggName(LanguageJapanese, 3)
		}
		encodeG3(p.data[0x08:0x12], name, 10, p.Language(), clearNone)
	default:
		if !p.IsNicknamed() && p.Nickname() == name {
			return
		}
		encodeGB(p.nick, name, len(p.nick), p.format, p.gbLang(), keepTerminatorMode(p.nick))
	}
}

// nonNickname returns the encoded default name for a language.
func (p *Pokemon) nonNickname(lang Language) []byte {
	buf := make([]byte, len(p.nick))
	name := defaultSpeciesNickname(p.Species(), lang, p.format)
	encodeGB(buf, name, len(buf), p.format, p.gbLang(), clear50)
	for i, b := range buf { // decimal point <-> period fix
		if b == 0xF2 {
			buf[i] = 0xE8
		}
	}
	return buf
}

// IsNicknamed reports whether the name differs from the species default.
func (p *Pokemon) IsNicknamed() bool {
	if p.format == 3 {
		return p.nickString() != defaultSpeciesNickname(p.Species(), p.Language(), 3)
	}
	expect := p.nonNickname(p.guessedLanguage())
	for i := range expect {
		if expect[i] != p.nick[i] {
			return true
		}
	}
	return false
}

// SetNotNicknamed resets the nickname to the species default in a language.
func (p *Pokemon) SetNotNicknamed(lang Language) {
	if p.format == 3 {
		p.SetNickname(defaultSpeciesNickname(p.Species(), lang, 3))
		return
	}
	copy(p.nick, p.nonNickname(lang))
}

// OTName returns the original trainer name.
func (p *Pokemon) OTName() string {
	switch p.format {
	case 1:
		return decodeGB(p.ot, 1, LanguageEnglish)
	case 2:
		return decodeGB(p.ot, 2, p.guessedLanguage())
	}
	return decodeG3(p.data[0x14:0x1B], p.Language())
}

// SetOTName changes the original trainer name.
func (p *Pokemon) SetOTName(name string) {
	if p.format == 3 {
		encodeG3(p.data[0x14:0x1B], name, 7, p.Language(), clearNone)
		return
	}
	if p.OTName() == name {
		return
	}
	encodeGB(p.ot, name, len(p.ot), p.format, p.gbLang(), keepTerminatorMode(p.ot))
}

// ---------------------------------------------------------------------------
// Trainer ids

// TID returns the trainer id.
func (p *Pokemon) TID() int {
	switch p.format {
	case 1:
		return be16(p.data[0xC:])
	case 2:
		return be16(p.data[6:])
	}
	return le16(p.data[4:])
}

// SetTID sets the trainer id.
func (p *Pokemon) SetTID(v int) {
	switch p.format {
	case 1:
		putBE16(p.data[0xC:], v)
	case 2:
		putBE16(p.data[6:], v)
	default:
		putLE16(p.data[4:], v)
	}
}

// SID returns the secret id (generation 3; 0 otherwise).
func (p *Pokemon) SID() int {
	if p.format == 3 {
		return le16(p.data[6:])
	}
	return 0
}

// SetSID sets the secret id (generation 3 only).
func (p *Pokemon) SetSID(v int) {
	if p.format == 3 {
		putLE16(p.data[6:], v)
	}
}

// PID returns the personality value (generation 3; 0 otherwise).
func (p *Pokemon) PID() uint32 {
	if p.format == 3 {
		return le32(p.data)
	}
	return 0
}

// SetPID sets the personality value (generation 3 only).
func (p *Pokemon) SetPID(v uint32) {
	if p.format == 3 {
		putLE32(p.data, v)
	}
}

// ---------------------------------------------------------------------------
// Experience and level

// EXP returns the experience points.
func (p *Pokemon) EXP() uint32 {
	switch p.format {
	case 1:
		return binary.BigEndian.Uint32(p.data[0xE:]) >> 8
	case 2:
		return binary.BigEndian.Uint32(p.data[0x8:]) >> 8
	}
	return le32(p.data[0x24:])
}

// SetEXP sets the experience points.
func (p *Pokemon) SetEXP(v uint32) {
	switch p.format {
	case 1:
		binary.BigEndian.PutUint32(p.data[0xE:], v<<8|uint32(p.data[0x11]))
	case 2:
		binary.BigEndian.PutUint32(p.data[0x8:], v<<8|uint32(p.data[0xB]))
	default:
		putLE32(p.data[0x24:], v)
	}
}

// Growth returns the species' experience growth rate.
func (p *Pokemon) Growth() int { return int(p.personal().EXPGrowth) }

// Level is the level implied by the experience points.
func (p *Pokemon) Level() int { return LevelForExp(p.EXP(), p.Growth()) }

// SetLevel sets the level and the matching minimum experience.
func (p *Pokemon) SetLevel(level int) {
	p.SetStatLevel(level)
	p.SetEXP(ExpForLevel(level, p.Growth()))
}

// StatLevel is the level cached with the party stats.
func (p *Pokemon) StatLevel() int {
	switch p.format {
	case 1:
		return int(p.data[0x21])
	case 2:
		return int(p.data[0x1F])
	}
	return int(p.data[0x54])
}

// SetStatLevel sets the level cached with the party stats.
func (p *Pokemon) SetStatLevel(level int) {
	switch p.format {
	case 1:
		p.data[0x21] = byte(level)
		p.data[3] = byte(level) // box level
	case 2:
		p.data[0x1F] = byte(level)
	default:
		p.data[0x54] = byte(level)
	}
}

// ---------------------------------------------------------------------------
// Party stats

// HP returns the current hit points.
func (p *Pokemon) HP() int {
	switch p.format {
	case 1:
		return be16(p.data[1:])
	case 2:
		return be16(p.data[0x22:])
	}
	return le16(p.data[0x56:])
}

// SetHP sets the current hit points.
func (p *Pokemon) SetHP(v int) {
	switch p.format {
	case 1:
		putBE16(p.data[1:], v)
	case 2:
		putBE16(p.data[0x22:], v)
	default:
		putLE16(p.data[0x56:], v)
	}
}

var partyStatOffsets = [4][6]int{
	1: {0x22, 0x24, 0x26, 0x28, 0x2A, 0x2A},
	2: {0x24, 0x26, 0x28, 0x2A, 0x2C, 0x2E},
	3: {0x58, 0x5A, 0x5C, 0x5E, 0x60, 0x62},
}

// Stat returns a stored party stat (HP is the maximum). Order: HP, Attack,
// Defense, Speed, Special Attack, Special Defense.
func (p *Pokemon) Stat(i int) int {
	o := partyStatOffsets[p.format][i]
	if p.format == 3 {
		return le16(p.data[o:])
	}
	return be16(p.data[o:])
}

// SetStat writes a stored party stat. Generation 1 has a single Special stat.
func (p *Pokemon) SetStat(i, v int) {
	o := partyStatOffsets[p.format][i]
	if p.format == 1 && i == StatSpecialDefense {
		return
	}
	if p.format == 3 {
		putLE16(p.data[o:], v)
		return
	}
	putBE16(p.data[o:], v)
}

// MaxHP returns the stored maximum hit points.
func (p *Pokemon) MaxHP() int { return p.Stat(StatHP) }

// Stats returns the stored party stats.
func (p *Pokemon) Stats() [6]int {
	var s [6]int
	for i := range s {
		s[i] = p.Stat(i)
	}
	return s
}

// SetStats stores calculated stats and fills HP.
func (p *Pokemon) SetStats(s [6]int) {
	p.SetStat(StatHP, s[0])
	p.SetHP(s[0])
	for i := 1; i < 6; i++ {
		p.SetStat(i, s[i])
	}
}

// PartyStatsPresent reports whether the party-only fields hold data.
func (p *Pokemon) PartyStatsPresent() bool { return p.MaxHP() != 0 }

// Status returns the status condition bits.
func (p *Pokemon) Status() int {
	switch p.format {
	case 1:
		return int(p.data[4])
	case 2:
		return int(p.data[0x20])
	}
	return int(int32(le32(p.data[0x50:])))
}

// SetStatus sets the status condition bits.
func (p *Pokemon) SetStatus(v int) {
	switch p.format {
	case 1:
		p.data[4] = byte(v)
	case 2:
		p.data[0x20] = byte(v)
	default:
		putLE32(p.data[0x50:], uint32(v))
	}
}

func gbStat(base, iv, ev, level int) int {
	root := int(math.Ceil(math.Sqrt(float64(ev))))
	if root > 255 {
		root = 255
	}
	return ((2*(base+iv)+(root>>2))*level)/100 + 5
}

// CalcStats computes stats from a personal entry, the way the games do.
func (p *Pokemon) CalcStats(base *Personal) [6]int {
	level := p.Level()
	var s [6]int
	if p.format == 3 {
		bases := [6]int{base.HP, base.ATK, base.DEF, base.SPE, base.SPA, base.SPD}
		for i := range s {
			if i == StatHP {
				if base.HP == 1 {
					s[i] = 1
				} else {
					s[i] = (p.IV(i)+2*bases[i]+p.EV(i)/4+100)*level/100 + 10
				}
				continue
			}
			s[i] = (p.IV(i)+2*bases[i]+p.EV(i)/4)*level/100 + 5
		}
		if n := p.Nature(); n >= 0 && n < 25 {
			up, dn := n/5, n%5
			if up != dn {
				s[up+1] = s[up+1] * 11 / 10
				s[dn+1] = s[dn+1] * 9 / 10
			}
		}
		return s
	}
	s[0] = gbStat(base.HP, p.IV(StatHP), p.EV(StatHP), level) + 5 + level
	s[1] = gbStat(base.ATK, p.IV(StatAttack), p.EV(StatAttack), level)
	s[2] = gbStat(base.DEF, p.IV(StatDefense), p.EV(StatDefense), level)
	s[3] = gbStat(base.SPE, p.IV(StatSpeed), p.EV(StatSpeed), level)
	s[4] = gbStat(base.SPA, p.IV(StatSpecialAttack), p.EV(StatSpecialAttack), level)
	s[5] = gbStat(base.SPD, p.IV(StatSpecialDefense), p.EV(StatSpecialDefense), level)
	return s
}

// ResetPartyStats recalculates stats at full health and clears the status.
func (p *Pokemon) ResetPartyStats() {
	p.SetStats(p.CalcStats(p.personal()))
	p.SetStatLevel(p.Level())
	p.SetStatus(0)
}

// RecalculateStats recalculates stats with a given personal entry, keeping the
// current HP (clamped) and the status condition.
func (p *Pokemon) RecalculateStats(base *Personal) {
	hp, status := p.HP(), p.Status()
	p.SetStats(p.CalcStats(base))
	p.SetStatLevel(p.Level())
	if hp > p.MaxHP() {
		hp = p.MaxHP()
	}
	p.SetHP(hp)
	p.SetStatus(status)
}

// ---------------------------------------------------------------------------
// Moves

var moveOffsets = [4]int{1: 8, 2: 2, 3: 0x2C}
var ppOffsets = [4]int{1: 0x1D, 2: 0x17, 3: 0x34}

// Move returns move slot i (0–3).
func (p *Pokemon) Move(i int) int {
	if p.format == 3 {
		return le16(p.data[0x2C+i*2:])
	}
	return int(p.data[moveOffsets[p.format]+i])
}

// SetMove sets move slot i. PP is not changed; see HealPP.
func (p *Pokemon) SetMove(i, move int) {
	if p.format == 3 {
		putLE16(p.data[0x2C+i*2:], move)
		return
	}
	p.data[moveOffsets[p.format]+i] = byte(move)
}

// Moves returns the four move slots.
func (p *Pokemon) Moves() [4]int {
	return [4]int{p.Move(0), p.Move(1), p.Move(2), p.Move(3)}
}

// MovePP returns the remaining PP of slot i.
func (p *Pokemon) MovePP(i int) int {
	if p.format == 3 {
		return int(p.data[0x34+i])
	}
	return int(p.data[ppOffsets[p.format]+i] & 0x3F)
}

// SetMovePP sets the remaining PP of slot i.
func (p *Pokemon) SetMovePP(i, v int) {
	if p.format == 3 {
		p.data[0x34+i] = byte(v)
		return
	}
	if v > 63 {
		v = 63
	}
	o := ppOffsets[p.format] + i
	p.data[o] = p.data[o]&0xC0 | byte(v)
}

// MovePPUps returns the PP Ups applied to slot i.
func (p *Pokemon) MovePPUps(i int) int {
	if p.format == 3 {
		return int(p.data[0x28]>>(uint(i)*2)) & 3
	}
	return int(p.data[ppOffsets[p.format]+i] >> 6)
}

// SetMovePPUps sets the PP Ups of slot i.
func (p *Pokemon) SetMovePPUps(i, v int) {
	if p.format == 3 {
		shift := uint(i) * 2
		p.data[0x28] = p.data[0x28]&^(3<<shift) | byte(v&3)<<shift
		return
	}
	o := ppOffsets[p.format] + i
	p.data[o] = p.data[o]&0x3F | byte(v&3)<<6
}

// MaxMovePP returns the full PP of a move with PP Ups applied.
func (p *Pokemon) MaxMovePP(move, ups int) int {
	base := BasePP(p.format, move)
	if p.format == 3 {
		return base * (5 + ups) / 5
	}
	step := base / 5
	if step > 7 {
		step = 7
	}
	return base + ups*step
}

// HealPP restores the PP of every move.
func (p *Pokemon) HealPP() {
	for i := 0; i < 4; i++ {
		p.SetMovePP(i, p.MaxMovePP(p.Move(i), p.MovePPUps(i)))
	}
}

// ---------------------------------------------------------------------------
// IVs and EVs

func (p *Pokemon) dv16() int {
	if p.format == 1 {
		return be16(p.data[0x1B:])
	}
	return be16(p.data[0x15:])
}

func (p *Pokemon) setDV16(v int) {
	if p.format == 1 {
		putBE16(p.data[0x1B:], v)
		return
	}
	putBE16(p.data[0x15:], v)
}

// DV16 returns the packed generation 1/2 DVs (0 for generation 3).
func (p *Pokemon) DV16() int {
	if p.format == 3 {
		return 0
	}
	return p.dv16()
}

// SetDV16 sets the packed generation 1/2 DVs.
func (p *Pokemon) SetDV16(v int) {
	if p.format != 3 {
		p.setDV16(v)
	}
}

// IV returns an IV/DV by stat index. Generation 1/2 report DVs (0–15), where
// HP is derived and Special Attack/Defense share one value.
func (p *Pokemon) IV(i int) int {
	if p.format == 3 {
		return int(le32(p.data[0x48:])>>(uint(i)*5)) & 0x1F
	}
	dv := p.dv16()
	atk, def, spe, spc := (dv>>12)&0xF, (dv>>8)&0xF, (dv>>4)&0xF, dv&0xF
	switch i {
	case StatHP:
		return (atk&1)<<3 | (def&1)<<2 | (spe&1)<<1 | spc&1
	case StatAttack:
		return atk
	case StatDefense:
		return def
	case StatSpeed:
		return spe
	}
	return spc
}

// MaxIV is the largest IV the format can hold.
func (p *Pokemon) MaxIV() int {
	if p.format == 3 {
		return 31
	}
	return 15
}

// SetIV sets an IV/DV. HP and Special Defense are derived in generations 1/2
// and ignore writes.
func (p *Pokemon) SetIV(i, v int) {
	if v > p.MaxIV() {
		v = p.MaxIV()
	}
	if v < 0 {
		v = 0
	}
	if p.format == 3 {
		shift := uint(i) * 5
		iv := le32(p.data[0x48:])
		putLE32(p.data[0x48:], iv&^(0x1F<<shift)|uint32(v)<<shift)
		return
	}
	var shift uint
	switch i {
	case StatAttack:
		shift = 12
	case StatDefense:
		shift = 8
	case StatSpeed:
		shift = 4
	case StatSpecialAttack:
		shift = 0
	default:
		return
	}
	p.setDV16(p.dv16()&^(0xF<<shift) | v<<shift)
}

var evOffsets = [4][5]int{1: {0x11, 0x13, 0x15, 0x17, 0x19}, 2: {0x0B, 0x0D, 0x0F, 0x11, 0x13}}

// MaxEV is the largest EV/stat experience value the format can hold.
func (p *Pokemon) MaxEV() int {
	if p.format == 3 {
		return 255
	}
	return 65535
}

// EV returns an EV (generation 3) or stat experience (generations 1/2).
func (p *Pokemon) EV(i int) int {
	if p.format == 3 {
		return int(p.data[0x38+i])
	}
	if i == StatSpecialDefense {
		i = StatSpecialAttack
	}
	return be16(p.data[evOffsets[p.format][i]:])
}

// SetEV sets an EV or stat experience value.
func (p *Pokemon) SetEV(i, v int) {
	if p.format == 3 {
		p.data[0x38+i] = byte(v)
		return
	}
	if i == StatSpecialDefense {
		return
	}
	putBE16(p.data[evOffsets[p.format][i]:], v)
}

// EVTotal sums all EVs of a generation 3 Pokémon.
func (p *Pokemon) EVTotal() int {
	t := 0
	for i := 0; i < 6; i++ {
		t += p.EV(i)
	}
	return t
}

// ---------------------------------------------------------------------------
// Misc fields

// HeldItem returns the held item id (generation 2/3; 0 in generation 1).
func (p *Pokemon) HeldItem() int {
	switch p.format {
	case 2:
		return int(p.data[1])
	case 3:
		return le16(p.data[0x22:])
	}
	return 0
}

// SetHeldItem sets the held item id.
func (p *Pokemon) SetHeldItem(v int) {
	switch p.format {
	case 2:
		p.data[1] = byte(v)
	case 3:
		putLE16(p.data[0x22:], v)
	}
}

// Friendship returns the friendship (generation 2/3; 0 in generation 1).
func (p *Pokemon) Friendship() int {
	switch p.format {
	case 2:
		return int(p.data[0x1B])
	case 3:
		return int(p.data[0x29])
	}
	return 0
}

// SetFriendship sets the friendship.
func (p *Pokemon) SetFriendship(v int) {
	switch p.format {
	case 2:
		p.data[0x1B] = byte(v)
	case 3:
		p.data[0x29] = byte(v)
	}
}

// IsEgg reports whether this is an unhatched egg.
func (p *Pokemon) IsEgg() bool {
	switch p.format {
	case 2:
		return p.egg
	case 3:
		return le32(p.data[0x48:])>>30&1 == 1
	}
	return false
}

// SetEgg marks or unmarks the Pokémon as an egg. Generation 3 eggs are renamed
// "タマゴ" and flagged Japanese, like the games.
func (p *Pokemon) SetEgg(v bool) {
	switch p.format {
	case 2:
		p.egg = v
	case 3:
		iv := le32(p.data[0x48:]) &^ 0x40000000
		if v {
			iv |= 0x40000000
			p.data[0x13] |= 4
		} else {
			p.data[0x13] &^= 4
		}
		putLE32(p.data[0x48:], iv)
		if v {
			p.data[0x13] |= 4
			encodeG3(p.data[0x08:0x12], eggName(LanguageJapanese, 3), 10, LanguageJapanese, clearNone)
			p.data[0x12] = byte(LanguageJapanese)
		}
	}
}

// Nature returns the generation 3 nature index (PID % 25), or -1 otherwise.
func (p *Pokemon) Nature() int {
	if p.format != 3 {
		return -1
	}
	return int(p.PID() % 25)
}

// Gender returns 0 male, 1 female, 2 genderless.
func (p *Pokemon) Gender() int {
	gr := int(p.personal().Gender)
	switch gr {
	case genderless:
		return 2
	case genderFemaleOnly:
		return 1
	case genderMale:
		return 0
	}
	if p.format == 3 {
		if int(p.PID()&0xFF) >= gr {
			return 0
		}
		return 1
	}
	if p.IV(StatAttack) > gr>>4 {
		return 0
	}
	return 1
}

// IsShiny reports whether the Pokémon is shiny.
func (p *Pokemon) IsShiny() bool {
	if p.format == 3 {
		pid := p.PID()
		return (p.TID() ^ p.SID() ^ int(pid>>16) ^ int(pid&0xFFFF)) < 8
	}
	return p.IV(StatAttack)&2 != 0 && p.IV(StatDefense) == 10 && p.IV(StatSpeed) == 10 && p.IV(StatSpecialAttack) == 10
}

// ContestBeauty returns the generation 3 beauty contest condition.
func (p *Pokemon) ContestBeauty() int {
	if p.format == 3 {
		return int(p.data[0x3F])
	}
	return 0
}

// CatchRate returns the generation 1 catch rate byte (doubles as a held item
// when the record is converted to generation 2).
func (p *Pokemon) CatchRate() int {
	if p.format == 1 {
		return int(p.data[7])
	}
	return 0
}

// MetLocation returns the met location (generation 2 Crystal / generation 3).
func (p *Pokemon) MetLocation() int {
	switch p.format {
	case 2:
		return int(be16(p.data[0x1D:]) & 0x7F)
	case 3:
		return int(p.data[0x45])
	}
	return 0
}

// SetMetLocation sets the met location.
func (p *Pokemon) SetMetLocation(v int) {
	switch p.format {
	case 2:
		putBE16(p.data[0x1D:], be16(p.data[0x1D:])&0xFF80|v&0x7F)
	case 3:
		p.data[0x45] = byte(v)
	}
}

// MetLevel returns the level at which the Pokémon was met.
func (p *Pokemon) MetLevel() int {
	switch p.format {
	case 2:
		return (be16(p.data[0x1D:]) >> 8) & 0x3F
	case 3:
		return le16(p.data[0x46:]) & 0x7F
	}
	return 0
}

// SetMetLevel sets the level at which the Pokémon was met.
func (p *Pokemon) SetMetLevel(v int) {
	switch p.format {
	case 2:
		putBE16(p.data[0x1D:], be16(p.data[0x1D:])&0xC0FF|(v&0x3F)<<8)
	case 3:
		putLE16(p.data[0x46:], le16(p.data[0x46:])&^0x7F|v&0x7F)
	}
}

// OriginGame returns the generation 3 origin game id (1 Sapphire, 2 Ruby,
// 3 Emerald, 4 FireRed, 5 LeafGreen), or 0.
func (p *Pokemon) OriginGame() int {
	if p.format == 3 {
		return (le16(p.data[0x46:]) >> 7) & 0xF
	}
	return 0
}

// SetOriginGame sets the generation 3 origin game id.
func (p *Pokemon) SetOriginGame(v int) {
	if p.format == 3 {
		putLE16(p.data[0x46:], le16(p.data[0x46:])&^0x780|(v&0xF)<<7)
	}
}

// Ball returns the generation 3 ball id.
func (p *Pokemon) Ball() int {
	if p.format == 3 {
		return (le16(p.data[0x46:]) >> 11) & 0xF
	}
	return 0
}

// SetBall sets the generation 3 ball id.
func (p *Pokemon) SetBall(v int) {
	if p.format == 3 {
		putLE16(p.data[0x46:], le16(p.data[0x46:])&^0x7800|(v&0xF)<<11)
	}
}

// OTGender returns the original trainer gender (generation 2 Crystal / 3).
func (p *Pokemon) OTGender() int {
	switch p.format {
	case 2:
		return (be16(p.data[0x1D:]) >> 7) & 1
	case 3:
		return (le16(p.data[0x46:]) >> 15) & 1
	}
	return 0
}

// SetOTGender sets the original trainer gender.
func (p *Pokemon) SetOTGender(v int) {
	switch p.format {
	case 2:
		putBE16(p.data[0x1D:], be16(p.data[0x1D:])&0xFF7F|(v&1)<<7)
	case 3:
		putLE16(p.data[0x46:], le16(p.data[0x46:])&^0x8000|(v&1)<<15)
	}
}

// AbilityBit returns the generation 3 ability slot bit.
func (p *Pokemon) AbilityBit() bool { return p.format == 3 && le32(p.data[0x48:])>>31 == 1 }

// SetAbilityBit sets the generation 3 ability slot bit.
func (p *Pokemon) SetAbilityBit(v bool) {
	if p.format != 3 {
		return
	}
	iv := le32(p.data[0x48:]) & 0x7FFFFFFF
	if v {
		iv |= 1 << 31
	}
	putLE32(p.data[0x48:], iv)
}

// HeldMailID returns the generation 3 attached mail slot (-1 for none).
func (p *Pokemon) HeldMailID() int {
	if p.format == 3 {
		return int(int8(p.data[0x55]))
	}
	return -1
}

func (p *Pokemon) setHeldMailID(v int) {
	if p.format == 3 {
		p.data[0x55] = byte(int8(v))
	}
}

// ---------------------------------------------------------------------------
// Validity

// Valid reports whether the record looks like a real Pokémon of its format.
func (p *Pokemon) Valid() bool {
	switch p.format {
	case 1:
		return p.Species() <= maxSpecies1 && (p.data[0] == 0 || p.Species() != 0)
	case 2:
		return p.Species() <= maxSpecies2
	}
	return true
}

func add16(b []byte) uint16 {
	var sum uint16
	for i := 0; i+1 < len(b); i += 2 {
		sum += binary.LittleEndian.Uint16(b[i:])
	}
	return sum
}

// ChecksumValid reports whether the generation 3 checksum matches (always true
// for earlier generations).
func (p *Pokemon) ChecksumValid() bool {
	if p.format != 3 {
		return true
	}
	return add16(p.data[0x20:0x50]) == uint16(le16(p.data[0x1C:]))
}

// RefreshChecksum recomputes the generation 3 checksum.
func (p *Pokemon) RefreshChecksum() {
	if p.format != 3 {
		return
	}
	p.data[0x13] &^= 1 // not a bad egg
	putLE16(p.data[0x1C:], int(add16(p.data[0x20:0x50])))
}
