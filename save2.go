package pokeedit

import "encoding/binary"

type sav2Offsets struct {
	options, trainer1, rival, timePlayed, palette, money, badges int
	currentBoxIndex, boxNames, party, dexCaught, dexSeen         int
	currentBox, gender, checksumEnd, checksum1, checksum2        int
	eventFlag                                                    int
	pouch                                                        gen2PouchOffsets
	daycare                                                      int
}

func newSav2Offsets(jp, crystal bool) *sav2Offsets {
	o := &sav2Offsets{options: 0x2000, trainer1: 0x2009, gender: -1}
	if jp {
		o.rival, o.timePlayed, o.palette, o.currentBox = 0x2017, 0x2034, 0x204C, 0x2D10
		if crystal {
			o.money, o.badges, o.currentBoxIndex, o.boxNames = 0x23BE, 0x23C7, 0x26E2, 0x26E5
			o.party, o.dexCaught, o.dexSeen, o.gender = 0x281A, 0x29AA, 0x29CA, 0x8000
			o.checksumEnd, o.checksum1, o.checksum2 = 0x2AE2, 0x2D0D, 0x7F0D
			o.pouch = gen2PouchOffsets{tm: 0x23C9, item: 0x2402, key: 0x242C, ball: 0x2447, pc: 0x2461}
			o.eventFlag = o.currentBoxIndex - 0x100
		} else {
			o.money, o.badges, o.currentBoxIndex, o.boxNames = 0x23BC, 0x23C5, 0x2705, 0x2708
			o.party, o.dexCaught, o.dexSeen = 0x283E, 0x29CE, 0x29EE
			o.checksumEnd, o.checksum1, o.checksum2 = 0x2C8B, 0x2D0D, 0x7F0D
			o.pouch = gen2PouchOffsets{tm: 0x23C7, item: 0x2400, key: 0x242A, ball: 0x2445, pc: 0x245F}
			o.eventFlag = o.currentBoxIndex - 0x105
		}
	} else {
		o.rival = 0x2021
		if crystal {
			o.timePlayed, o.palette, o.money, o.badges = 0x2052, 0x206A, 0x23DC, 0x23E5
			o.currentBoxIndex, o.boxNames, o.party = 0x2700, 0x2703, 0x2865
			o.dexCaught, o.dexSeen, o.currentBox, o.gender = 0x2A27, 0x2A47, 0x2D10, 0x3E3D
			o.checksumEnd, o.checksum1, o.checksum2 = 0x2B82, 0x2D0D, 0x1F0D
			o.pouch = gen2PouchOffsets{tm: 0x23E7, item: 0x2420, key: 0x244A, ball: 0x2465, pc: 0x247F}
			o.eventFlag = o.currentBoxIndex - 0x100
		} else {
			o.timePlayed, o.palette, o.money, o.badges = 0x2053, 0x206B, 0x23DB, 0x23E4
			o.currentBoxIndex, o.boxNames, o.party = 0x2724, 0x2727, 0x288A
			o.dexCaught, o.dexSeen, o.currentBox = 0x2A4C, 0x2A6C, 0x2D6C
			o.checksumEnd, o.checksum1, o.checksum2 = 0x2D68, 0x2D69, 0x7E6D
			o.pouch = gen2PouchOffsets{tm: 0x23E6, item: 0x241F, key: 0x2449, ball: 0x2464, pc: 0x247E}
			o.eventFlag = o.currentBoxIndex - 0x105
		}
	}
	o.daycare = o.dexSeen + 0x1F + 28 + 1
	return o
}

type gen2Info struct {
	jp      bool
	crystal bool
}

func detectGen2(data []byte) (gen2Info, bool) {
	if len(data) != sizeG2RawU && len(data) != sizeG2RawJ {
		return gen2Info{}, false
	}
	has := func(a, b, max int) bool { return listValid(data, a, max) && listValid(data, b, max) }
	switch {
	case has(0x288A, 0x2D6C, 20):
		return gen2Info{}, true
	case has(0x2865, 0x2D10, 20):
		return gen2Info{crystal: true}, true
	case has(0x2D10, 0x283E, 30):
		return gen2Info{jp: true}, true
	case has(0x2D10, 0x281A, 30):
		return gen2Info{jp: true, crystal: true}, true
	}
	return gen2Info{}, false
}

// Save2 is a Gold/Silver/Crystal save.
type Save2 struct {
	data    []byte
	footer  []byte
	jp      bool
	crystal bool
	game    Game
	o       *sav2Offsets
	party   []*Pokemon
	boxes   [][]*Pokemon
	// occupied counts at load/flush time, used to clear vacated slots
	partyPrev int
	boxPrev   []int
}

func openSave2(data []byte, info gen2Info) (*Save2, error) {
	s := &Save2{data: data, jp: info.jp, crystal: info.crystal, game: GoldSilver}
	if info.crystal {
		s.game = Crystal
	}
	s.o = newSav2Offsets(s.jp, s.crystal)
	s.load()
	return s, nil
}

func (s *Save2) capacity() int {
	if s.jp {
		return 30
	}
	return 20
}

func (s *Save2) boxListLen() int   { return gbListLength(2, s.capacity(), s.jp, false) }
func (s *Save2) partyListLen() int { return gbListLength(2, 6, s.jp, true) }

func (s *Save2) rawBoxOffset(i int) int {
	split := 7
	if s.jp {
		split = 6
	}
	step := s.boxListLen() + 2
	if i < split {
		return 0x4000 + i*step
	}
	return 0x6000 + (i-split)*step
}

func (s *Save2) load() {
	s.boxes = make([][]*Pokemon, s.BoxCount())
	for i := range s.boxes {
		o := s.rawBoxOffset(i)
		s.boxes[i] = unpackGBList(s.data[o:o+s.boxListLen()], 2, s.capacity(), s.jp, false)
	}
	s.party = unpackGBList(s.data[s.o.party:s.o.party+s.partyListLen()], 2, 6, s.jp, true)
	s.markLoaded()
}

func (s *Save2) markLoaded() {
	s.partyPrev = len(s.party)
	s.boxPrev = make([]int, len(s.boxes))
	for i, b := range s.boxes {
		s.boxPrev[i] = len(b)
	}
}

func (s *Save2) Game() Game { return s.game }

// SetGame selects Gold, Silver or Crystal; the choice must match the save.
func (s *Save2) SetGame(g Game) error {
	ok := (g == Crystal) == s.crystal && g.Generation() == 2
	if err := require(ok, "Selected game does not match the detected save format."); err != nil {
		return err
	}
	s.game = g
	return nil
}

func (s *Save2) Generation() int { return 2 }
func (s *Save2) Japanese() bool  { return s.jp }

// Language returns English, or Japanese for Japanese saves.
func (s *Save2) Language() Language {
	if s.jp {
		return LanguageJapanese
	}
	return LanguageEnglish
}

func (s *Save2) checksum() uint16 {
	var sum uint16
	for i := s.o.trainer1; i <= s.o.checksumEnd; i++ {
		sum += uint16(s.data[i])
	}
	return sum
}

func (s *Save2) ChecksumsValid() bool {
	sum := s.checksum()
	return binary.LittleEndian.Uint16(s.data[s.o.checksum1:]) == sum &&
		binary.LittleEndian.Uint16(s.data[s.o.checksum2:]) == sum
}

func (s *Save2) Bytes() ([]byte, error) {
	s.flush()
	return append(append([]byte(nil), s.data...), s.footer...), nil
}

func (s *Save2) flush() {
	for i, b := range s.boxes {
		o := s.rawBoxOffset(i)
		dest := s.data[o : o+s.boxListLen()]
		if mergeGBList(b, dest, 2, s.capacity(), s.jp, false, true, s.boxPrev[i]) && i == s.CurrentBox() {
			copy(s.data[s.o.currentBox:], dest)
		}
	}
	mergeGBList(s.party, s.data[s.o.party:s.o.party+s.partyListLen()], 2, 6, s.jp, true, true, s.partyPrev)
	sum := s.checksum()
	binary.LittleEndian.PutUint16(s.data[s.o.checksum1:], sum)
	binary.LittleEndian.PutUint16(s.data[s.o.checksum2:], sum)
	s.markLoaded()
	d := s.data
	switch {
	case s.jp && s.crystal:
		copy(d[0x7209:], d[s.o.trainer1:s.o.trainer1+0xADA])
	case s.jp:
		copy(d[0x7209:], d[s.o.trainer1:s.o.trainer1+0xC83])
	case s.crystal:
		copy(d[0x1209:], d[0x2009:0x2009+0xB7A])
	default:
		copy(d[0x15C7:], d[0x2009:0x222F])
		copy(d[0x3D69:], d[0x222F:0x23D9])
		copy(d[0x0C6B:], d[0x23D9:0x2856])
		copy(d[0x7E39:], d[0x2856:0x288A])
		copy(d[0x10E8:], d[0x288A:0x2D69])
	}
}

func (s *Save2) Clone() (Save, error) {
	raw, _ := s.Bytes()
	c, err := Open(raw, OpenOptions{AllowBadChecksums: true})
	if err != nil {
		return nil, err
	}
	_ = c.SetGame(s.game)
	return c, nil
}

// Trainer -----------------------------------------------------------------

func (s *Save2) MaxTrainerLength() int {
	if s.jp {
		return 5
	}
	return 7
}

func (s *Save2) MaxNicknameLength() int {
	if s.jp {
		return 5
	}
	return 10
}

func (s *Save2) OT() string {
	return decodeGB(s.data[s.o.trainer1+2:s.o.trainer1+2+s.MaxTrainerLength()], 2, s.Language())
}

func (s *Save2) setName(off int, name string) error {
	n := s.MaxTrainerLength()
	if err := require(len([]rune(name)) > 0 && len([]rune(name)) <= n, "Trainer/rival name is too long for this game."); err != nil {
		return err
	}
	encodeGB(s.data[off:off+n], name, 8, 2, s.Language(), clear50)
	return nil
}

func (s *Save2) SetOT(name string) error { return s.setName(s.o.trainer1+2, name) }

func (s *Save2) RivalName() (string, bool) {
	return decodeGB(s.data[s.o.rival:s.o.rival+s.MaxTrainerLength()], 2, s.Language()), true
}

func (s *Save2) SetRivalName(name string) error { return s.setName(s.o.rival, name) }

func (s *Save2) TID() int { return be16(s.data[s.o.trainer1:]) }
func (s *Save2) SetTID(v int) {
	putBE16(s.data[s.o.trainer1:], v)
}
func (s *Save2) SID() int      { return 0 }
func (s *Save2) SetSID(int)    {}
func (s *Save2) MaxMoney() int { return 999999 }

// Gender is the player's gender in Crystal (0 boy, 1 girl); always 0 elsewhere.
func (s *Save2) Gender() int {
	if !s.crystal || s.o.gender < 0 {
		return 0
	}
	return int(s.data[s.o.gender])
}

func (s *Save2) SetGender(v int) {
	if !s.crystal || s.o.gender < 0 {
		return
	}
	s.data[s.o.gender] = byte(v)
	s.data[s.o.palette] = byte(v)
}

func (s *Save2) Money() uint32 {
	b := s.data[s.o.money:]
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}

func (s *Save2) SetMoney(v uint32) error {
	if err := require(v <= 999999, "Money is out of range."); err != nil {
		return err
	}
	b := s.data[s.o.money:]
	b[0], b[1], b[2] = byte(v>>16), byte(v>>8), byte(v)
	return nil
}

func (s *Save2) Coins() uint32 { return uint32(be16(s.data[s.o.money+7:])) }

func (s *Save2) SetCoins(v uint32) error {
	if err := require(v <= 9999, "Coins are out of range."); err != nil {
		return err
	}
	putBE16(s.data[s.o.money+7:], int(v))
	return nil
}

func (s *Save2) Badges() int {
	return int(binary.LittleEndian.Uint16(s.data[s.o.badges:]))
}

func (s *Save2) SetBadges(v int) {
	binary.LittleEndian.PutUint16(s.data[s.o.badges:], uint16(v))
}

func (s *Save2) MaxSpecies() int        { return maxSpecies2 }
func (s *Save2) MaxMove() int           { return maxMove2 }
func (s *Save2) MaxIV() int             { return 15 }
func (s *Save2) MaxEV() int             { return 65535 }
func (s *Save2) BlankPokemon() *Pokemon { return NewPokemon(2, s.jp) }

func (s *Save2) HeldItems() []uint16 { return HeldItemIDs(Crystal) }

func (s *Save2) Personal() *PersonalTable {
	if s.crystal {
		return personalTable("c", 2)
	}
	return personalTable("gs", 2)
}

// Storage -------------------------------------------------------------------

func (s *Save2) PartyCount() int { return len(s.party) }

func (s *Save2) Party(i int) (*Pokemon, error) {
	if i < 0 || i >= len(s.party) {
		return nil, fail("Select an occupied Pokémon slot.")
	}
	return s.party[i].Clone(), nil
}

func (s *Save2) SetParty(i int, pk *Pokemon) error {
	if err := checkFormat(pk, 2, s.jp); err != nil {
		return err
	}
	if i < 0 || i > len(s.party) || i > 5 {
		return fail("Select an occupied Pokémon slot.")
	}
	pk = pk.Clone()
	if !pk.PartyStatsPresent() {
		pk.ResetPartyStats()
	}
	if i == len(s.party) {
		s.party = append(s.party, pk)
	} else {
		s.party[i] = pk
	}
	return nil
}

func (s *Save2) DeleteParty(i int) error {
	if i < 0 || i >= len(s.party) {
		return nil
	}
	s.party = append(s.party[:i], s.party[i+1:]...)
	return nil
}

func (s *Save2) BoxCount() int {
	if s.jp {
		return 9
	}
	return 14
}
func (s *Save2) BoxSlotCount() int { return s.capacity() }
func (s *Save2) CurrentBox() int   { return int(s.data[s.o.currentBoxIndex] & 0x7F) }

func (s *Save2) Box(box, slot int) (*Pokemon, error) {
	if box < 0 || box >= len(s.boxes) {
		return nil, fail("Invalid box.")
	}
	if slot < 0 || slot >= s.capacity() {
		return nil, fail("Select an occupied Pokémon slot.")
	}
	if slot >= len(s.boxes[box]) {
		return nil, nil
	}
	return s.boxes[box][slot].Clone(), nil
}

func (s *Save2) SetBox(box, slot int, pk *Pokemon) error {
	if err := checkFormat(pk, 2, s.jp); err != nil {
		return err
	}
	if box < 0 || box >= len(s.boxes) {
		return fail("Invalid box.")
	}
	b := s.boxes[box]
	if slot < 0 || slot > len(b) || slot >= s.capacity() {
		return fail("This box is full or locked.")
	}
	pk = pk.Clone()
	if slot == len(b) {
		s.boxes[box] = append(b, pk)
	} else {
		b[slot] = pk
	}
	return nil
}

func (s *Save2) DeleteBox(box, slot int) error {
	if box < 0 || box >= len(s.boxes) {
		return fail("Invalid box.")
	}
	b := s.boxes[box]
	if slot < 0 || slot >= len(b) {
		return nil
	}
	s.boxes[box] = append(b[:slot], b[slot+1:]...)
	return nil
}

func (s *Save2) FirstFreeBoxSlot(box int) int {
	if box < 0 || box >= len(s.boxes) || len(s.boxes[box]) >= s.capacity() {
		return -1
	}
	return len(s.boxes[box])
}

// Pokédex ---------------------------------------------------------------------

func (s *Save2) dexFlag(base, species int) bool {
	if species < 1 || species > maxSpecies2 {
		return false
	}
	bit := species - 1
	return getFlag(s.data, base+bit>>3, bit&7)
}

func (s *Save2) setDexFlag(base, species int, v bool) {
	if species < 1 || species > maxSpecies2 {
		return
	}
	bit := species - 1
	setFlag(s.data, base+bit>>3, bit&7, v)
}

func (s *Save2) Seen(species int) bool       { return s.dexFlag(s.o.dexSeen, species) }
func (s *Save2) Caught(species int) bool     { return s.dexFlag(s.o.dexCaught, species) }
func (s *Save2) SetSeen(species int, v bool) { s.setDexFlag(s.o.dexSeen, species, v) }

// SetCaught also unlocks every Unown form entry when Unown is caught, so the
// Pokédex does not crash on its Unown page.
func (s *Save2) SetCaught(species int, v bool) {
	s.setDexFlag(s.o.dexCaught, species, v)
	if v && species == 201 {
		for i := 1; i <= 26; i++ {
			s.data[s.o.dexSeen+0x1F+i] = byte(i)
		}
		if s.data[s.o.dexSeen+0x1F+28] == 0 {
			s.data[s.o.dexSeen+0x1F+28] = 1
		}
	}
}

// Inventory --------------------------------------------------------------------

func (s *Save2) defs() []pouchDef { return gen2Defs(s.crystal, s.o.pouch) }

func (s *Save2) Pockets() []Pocket { return pocketsOf(s.defs(), s.data) }

func (s *Save2) SetPocket(i int, entries []ItemStack) error {
	return setPocketIn(s.defs(), s.data, i, entries)
}
