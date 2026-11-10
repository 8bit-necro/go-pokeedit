package pokeedit

type sav1Offsets struct {
	dexCaught, dexSeen, items, money, rival, options, badges, tid  int
	pikaFriendship, pcItems, currentBoxIndex, coin, starter, party int
	currentBox, checksum                                           int
}

const sav1OT = 0x2598

var sav1Intl = sav1Offsets{
	dexCaught: 0x25A3, dexSeen: 0x25B6, items: 0x25C9, money: 0x25F3, rival: 0x25F6, options: 0x2601,
	badges: 0x2602, tid: 0x2605, pikaFriendship: 0x271C, pcItems: 0x27E6, currentBoxIndex: 0x284C,
	coin: 0x2850, starter: 0x29C3, party: 0x2F2C, currentBox: 0x30C0, checksum: 0x3523,
}

var sav1Japan = sav1Offsets{
	dexCaught: 0x259E, dexSeen: 0x25B1, items: 0x25C4, money: 0x25EE, rival: 0x25F1, options: 0x25F7,
	badges: 0x25F8, tid: 0x25FB, pikaFriendship: 0x2712, pcItems: 0x27DC, currentBoxIndex: 0x2842,
	coin: 0x2846, starter: 0x29B9, party: 0x2ED5, currentBox: 0x302D, checksum: 0x3594,
}

type gen1Info struct {
	jp     bool
	yellow bool
}

func detectGen1(data []byte) (gen1Info, bool) {
	if listValid(data, 0x2ED5, 30) && listValid(data, 0x302D, 30) {
		return gen1Info{jp: true, yellow: isYellow(data[0x29B9], data[0x2712])}, true
	}
	if listValid(data, 0x2F2C, 20) && listValid(data, 0x30C0, 20) {
		return gen1Info{yellow: isYellow(data[0x29C3], data[0x271C])}, true
	}
	return gen1Info{}, false
}

func isYellow(starter, friendship byte) bool {
	if starter != 0 {
		return starter == 0x54
	}
	return friendship != 0
}

// Save1 is a Red/Blue/Green/Yellow save.
type Save1 struct {
	data   []byte
	footer []byte
	jp     bool
	game   Game
	family Game // RedBlue or Yellow
	o      *sav1Offsets
	party  []*Pokemon
	boxes  [][]*Pokemon
	// occupied counts at load/flush time, used to clear vacated slots
	partyPrev int
	boxPrev   []int
	orig      []byte // file contents when opened, to detect changed box banks
}

func openSave1(data []byte, info gen1Info) (*Save1, error) {
	s := &Save1{data: data, jp: info.jp, o: &sav1Intl, family: RedBlue}
	if info.jp {
		s.o = &sav1Japan
	}
	if info.yellow {
		s.family = Yellow
	}
	s.game = s.family
	s.orig = append([]byte(nil), data...)
	s.load()
	return s, nil
}

func (s *Save1) boxCapacity() int {
	if s.jp {
		return 30
	}
	return 20
}

func (s *Save1) boxLen() int   { return gbListLength(1, s.boxCapacity(), s.jp, false) }
func (s *Save1) partyLen() int { return gbListLength(1, 6, s.jp, true) }

func (s *Save1) rawBoxOffset(box int) int {
	half := s.BoxCount() / 2
	if box < half {
		return 0x4000 + box*s.boxLen()
	}
	return 0x6000 + (box-half)*s.boxLen()
}

func (s *Save1) boxesInitialized() bool { return s.data[s.o.currentBoxIndex]&0x80 != 0 }

func (s *Save1) load() {
	cap := s.boxCapacity()
	s.boxes = make([][]*Pokemon, s.BoxCount())
	current := s.CurrentBox()
	if s.boxesInitialized() {
		for i := range s.boxes {
			if i == current {
				continue
			}
			o := s.rawBoxOffset(i)
			s.boxes[i] = unpackGBList(s.data[o:o+s.boxLen()], 1, cap, s.jp, false)
		}
	}
	if current < len(s.boxes) {
		o := s.o.currentBox
		s.boxes[current] = unpackGBList(s.data[o:o+s.boxLen()], 1, cap, s.jp, false)
	}
	s.party = unpackGBList(s.data[s.o.party:s.o.party+s.partyLen()], 1, 6, s.jp, true)
	s.markLoaded()
}

func (s *Save1) markLoaded() {
	s.partyPrev = len(s.party)
	s.boxPrev = make([]int, len(s.boxes))
	for i, b := range s.boxes {
		s.boxPrev[i] = len(b)
	}
}

// Game returns the selected game (Red/Blue family or Yellow).
func (s *Save1) Game() Game { return s.game }

// SetGame selects Red, Blue, Green or Yellow; the choice must match the save.
func (s *Save1) SetGame(g Game) error {
	ok := (g == Yellow) == (s.family == Yellow) && g.Generation() == 1
	if err := require(ok, "Selected game does not match the detected save format."); err != nil {
		return err
	}
	s.game = g
	return nil
}

func (s *Save1) Generation() int { return 1 }
func (s *Save1) Japanese() bool  { return s.jp }

// Language returns English, or Japanese for Japanese saves.
func (s *Save1) Language() Language {
	if s.jp {
		return LanguageJapanese
	}
	return LanguageEnglish
}

func (s *Save1) checksumOf() byte {
	var sum byte
	for _, b := range s.data[sav1OT:s.o.checksum] {
		sum += b
	}
	return ^sum
}

// ChecksumsValid verifies the main checksum.
func (s *Save1) ChecksumsValid() bool { return s.data[s.o.checksum] == s.checksumOf() }

// Bytes finalizes the save and returns the file contents.
func (s *Save1) Bytes() ([]byte, error) {
	s.flush()
	return append(append([]byte(nil), s.data...), s.footer...), nil
}

func (s *Save1) flush() {
	cap := s.boxCapacity()
	initialized := s.boxesInitialized()
	current := s.CurrentBox()
	if !initialized {
		content := false
		for i, b := range s.boxes {
			if i != current && len(b) > 0 {
				content = true
				break
			}
		}
		if content {
			s.data[s.o.currentBoxIndex] |= 0x80
			initialized = true
		} else {
			current = 0
			s.data[s.o.currentBoxIndex] &= 0x80
		}
	}
	for i, b := range s.boxes {
		o := s.rawBoxOffset(i)
		dest := s.data[o : o+s.boxLen()]
		written := mergeGBList(b, dest, 1, cap, s.jp, false, initialized, s.boxPrev[i])
		if i != current {
			continue
		}
		cur := s.data[s.o.currentBox : s.o.currentBox+s.boxLen()]
		if written {
			copy(cur, dest)
		} else {
			mergeGBList(b, cur, 1, cap, s.jp, false, initialized, s.boxPrev[i])
		}
	}
	mergeGBList(s.party, s.data[s.o.party:s.o.party+s.partyLen()], 1, 6, s.jp, true, true, s.partyPrev)
	s.updateBankChecksums()
	s.data[s.o.checksum] = s.checksumOf()
	s.markLoaded()
}

// Clone returns an independent copy of the save.
func (s *Save1) Clone() (Save, error) {
	raw, _ := s.Bytes()
	c, err := Open(raw, OpenOptions{AllowBadChecksums: true})
	if err != nil {
		return nil, err
	}
	_ = c.SetGame(s.game)
	return c, nil
}

// Trainer ---------------------------------------------------------------

// OT returns the player name.
func (s *Save1) OT() string {
	return decodeGB(s.data[sav1OT:sav1OT+s.MaxTrainerLength()], 1, s.Language())
}

// SetOT sets the player name.
func (s *Save1) SetOT(name string) error {
	return s.setName(sav1OT, name, clearZero)
}

func (s *Save1) setName(off int, name string, mode clearMode) error {
	n := s.MaxTrainerLength()
	if err := require(len([]rune(name)) > 0 && len([]rune(name)) <= n, "Trainer/rival name is too long for this game."); err != nil {
		return err
	}
	buf := s.data[off : off+n+boolInt(mode == clearZero)]
	encodeGB(buf, name, n, 1, s.Language(), mode)
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Save1) TID() int { return be16(s.data[s.o.tid:]) }
func (s *Save1) SetTID(v int) {
	putBE16(s.data[s.o.tid:], v)
}
func (s *Save1) SID() int        { return 0 }
func (s *Save1) SetSID(int)      {}
func (s *Save1) Gender() int     { return 0 }
func (s *Save1) SetGender(int)   {}
func (s *Save1) MaxMoney() int   { return 999999 }
func (s *Save1) Money() uint32   { return bcdRead(s.data[s.o.money : s.o.money+3]) }
func (s *Save1) Coins() uint32   { return bcdRead(s.data[s.o.coin : s.o.coin+2]) }
func (s *Save1) Badges() int     { return int(s.data[s.o.badges]) }
func (s *Save1) SetBadges(v int) { s.data[s.o.badges] = byte(v) }

// SetMoney sets the player's money (0–999999).
func (s *Save1) SetMoney(v uint32) error {
	if err := require(v <= 999999, "Money is out of range."); err != nil {
		return err
	}
	bcdWrite(s.data[s.o.money:s.o.money+3], v)
	return nil
}

// SetCoins sets the Game Corner coins (0–9999).
func (s *Save1) SetCoins(v uint32) error {
	if err := require(v <= 9999, "Coins are out of range."); err != nil {
		return err
	}
	bcdWrite(s.data[s.o.coin:s.o.coin+2], v)
	return nil
}

// RivalName returns the rival's name.
func (s *Save1) RivalName() (string, bool) {
	return decodeGB(s.data[s.o.rival:s.o.rival+s.MaxTrainerLength()], 1, s.Language()), true
}

// SetRivalName sets the rival's name.
func (s *Save1) SetRivalName(name string) error { return s.setName(s.o.rival, name, clear50) }

func (s *Save1) MaxSpecies() int        { return maxSpecies1 }
func (s *Save1) MaxMove() int           { return maxMove1 }
func (s *Save1) MaxIV() int             { return 15 }
func (s *Save1) MaxEV() int             { return 65535 }
func (s *Save1) HeldItems() []uint16    { return nil }
func (s *Save1) BlankPokemon() *Pokemon { return NewPokemon(1, s.jp) }
func (s *Save1) MaxNicknameLength() int {
	if s.jp {
		return 5
	}
	return 10
}
func (s *Save1) MaxTrainerLength() int {
	if s.jp {
		return 5
	}
	return 7
}

// Personal returns the game's personal table.
func (s *Save1) Personal() *PersonalTable {
	if s.family == Yellow {
		return personalTable("y", 1)
	}
	return personalTable("rb", 1)
}

// Storage ------------------------------------------------------------------

func (s *Save1) PartyCount() int { return len(s.party) }

func (s *Save1) Party(i int) (*Pokemon, error) {
	if i < 0 || i >= len(s.party) {
		return nil, fail("Select an occupied Pokémon slot.")
	}
	return s.party[i].Clone(), nil
}

func (s *Save1) SetParty(i int, pk *Pokemon) error {
	if err := checkFormat(pk, 1, s.jp); err != nil {
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

func (s *Save1) DeleteParty(i int) error {
	if i < 0 || i >= len(s.party) {
		return nil
	}
	s.party = append(s.party[:i], s.party[i+1:]...)
	return nil
}

func (s *Save1) BoxCount() int {
	if s.jp {
		return 8
	}
	return 12
}
func (s *Save1) BoxSlotCount() int { return s.boxCapacity() }
func (s *Save1) CurrentBox() int   { return int(s.data[s.o.currentBoxIndex] & 0x7F) }

func (s *Save1) Box(box, slot int) (*Pokemon, error) {
	if box < 0 || box >= len(s.boxes) {
		return nil, fail("Invalid box.")
	}
	if slot < 0 || slot >= s.boxCapacity() {
		return nil, fail("Select an occupied Pokémon slot.")
	}
	if slot >= len(s.boxes[box]) {
		return nil, nil
	}
	return s.boxes[box][slot].Clone(), nil
}

func (s *Save1) SetBox(box, slot int, pk *Pokemon) error {
	if err := checkFormat(pk, 1, s.jp); err != nil {
		return err
	}
	if box < 0 || box >= len(s.boxes) {
		return fail("Invalid box.")
	}
	b := s.boxes[box]
	if slot < 0 || slot > len(b) || slot >= s.boxCapacity() {
		return fail("This box is full or locked.")
	}
	pk = pk.Clone()
	pk.data[3] = byte(pk.Level()) // box level
	if slot == len(b) {
		s.boxes[box] = append(b, pk)
	} else {
		b[slot] = pk
	}
	return nil
}

func (s *Save1) DeleteBox(box, slot int) error {
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

func (s *Save1) FirstFreeBoxSlot(box int) int {
	if box < 0 || box >= len(s.boxes) || len(s.boxes[box]) >= s.boxCapacity() {
		return -1
	}
	return len(s.boxes[box])
}

// Pokédex -------------------------------------------------------------------

func (s *Save1) dexFlag(base, species int) bool {
	if species < 1 || species > maxSpecies1 {
		return false
	}
	bit := species - 1
	return getFlag(s.data, base+bit>>3, bit&7)
}

func (s *Save1) setDexFlag(base, species int, v bool) {
	if species < 1 || species > maxSpecies1 {
		return
	}
	bit := species - 1
	setFlag(s.data, base+bit>>3, bit&7, v)
}

func (s *Save1) Seen(species int) bool         { return s.dexFlag(s.o.dexSeen, species) }
func (s *Save1) Caught(species int) bool       { return s.dexFlag(s.o.dexCaught, species) }
func (s *Save1) SetSeen(species int, v bool)   { s.setDexFlag(s.o.dexSeen, species, v) }
func (s *Save1) SetCaught(species int, v bool) { s.setDexFlag(s.o.dexCaught, species, v) }

// Inventory ----------------------------------------------------------------

func (s *Save1) defs() []pouchDef { return gen1Defs(s.o.items, s.o.pcItems) }

func (s *Save1) Pockets() []Pocket { return pocketsOf(s.defs(), s.data) }

func (s *Save1) SetPocket(i int, entries []ItemStack) error {
	return setPocketIn(s.defs(), s.data, i, entries)
}

// Extras ---------------------------------------------------------------------

// Starter returns the starter id byte (0x54 is Yellow's Pikachu).
func (s *Save1) Starter() byte { return s.data[s.o.starter] }

// SetStarter sets the starter id byte.
func (s *Save1) SetStarter(v byte) { s.data[s.o.starter] = v }

func checkFormat(pk *Pokemon, format int, jp bool) error {
	if pk == nil || pk.format != format {
		return fail("Pokémon format is incompatible with this game.")
	}
	if format <= 2 && pk.Japanese() != jp {
		return fail("Japanese/international/Korean GB records cannot be mixed.")
	}
	return nil
}

// updateBankChecksums refreshes the checksums the international games keep
// after each bank of six PC boxes (one per box, then one over all six), for
// banks whose contents changed since the save was opened. Untouched banks are
// never rewritten.
func (s *Save1) updateBankChecksums() {
	if s.jp || s.orig == nil {
		return
	}
	size := s.boxLen()
	for _, base := range []int{0x4000, 0x6000} {
		end := base + 6*size
		if string(s.data[base:end]) == string(s.orig[base:end]) {
			continue
		}
		sum := func(b []byte) byte {
			var t byte
			for _, v := range b {
				t += v
			}
			return ^t
		}
		for i := 0; i < 6; i++ {
			s.data[end+1+i] = sum(s.data[base+i*size : base+(i+1)*size])
		}
		s.data[end] = sum(s.data[base:end])
	}
	copy(s.orig, s.data) // the file now matches what we wrote
}

// BoxesInitialized reports whether the game has initialized the PC boxes
// other than the current one (before that, only the current box is usable).
func (s *Save1) BoxesInitialized() bool { return s.boxesInitialized() }
