package pokeedit

import "encoding/binary"

type gen3Kind int

const (
	kindRS gen3Kind = iota
	kindE
	kindFRLG
)

const (
	sectorSize     = 0x1000
	sectorUsed     = 0xF80
	mainSectors    = 14
	extraSectors   = 4
	mainSize       = mainSectors * sectorSize
	extraSectorOfs = 0x1C000
)

type sav3Layout struct {
	smallLen, largeLen     int
	partyCount, partyBuf   int
	money, coin, inventory int
	seen2, seen3           int
	eventFlag, badgeFlag   int
	keyOffset              int // in the small block; -1 when unused
	rivalOffset            int // in the large block; -1 when unused
}

var sav3Layouts = [...]sav3Layout{
	kindRS:   {0x890, 0x3AC0, 0x234, 0x238, 0x490, 0x494, 0x498, 0x938, 0x3A8C, 0x1220, 0x807, -1, -1},
	kindE:    {0xF2C, 0x3D88, 0x234, 0x238, 0x490, 0x494, 0x498, 0x988, 0x3B24, 0x1270, 0x867, 0xAC, -1},
	kindFRLG: {0xF24, 0x3D68, 0x034, 0x038, 0x290, 0x294, 0x298, 0x5F8, 0x3A18, 0x0EE0, 0x820, 0xF20, 0x3A4C},
}

func sectorID(data []byte, ofs int) int {
	return int(int16(binary.LittleEndian.Uint16(data[ofs+0xFF4:])))
}

// allMainSectors reports whether a save slot has all 14 sectors, returning the
// file offset of sector 0.
func allMainSectors(data []byte, slot int) (sector0 int, ok bool) {
	start := slot * mainSize
	if start+mainSize > len(data) {
		return 0, false
	}
	bits := 0
	for ofs := start; ofs < start+mainSize; ofs += sectorSize {
		id := sectorID(data, ofs)
		if id < 0 || id >= mainSectors {
			return 0, false
		}
		bits |= 1 << id
		if id == 0 {
			sector0 = ofs
		}
	}
	return sector0, bits == 0x3FFF
}

func detectGen3(data []byte) (gen3Kind, bool) {
	if len(data) != sizeG3Raw && len(data) != sizeG3RawHalf {
		return 0, false
	}
	for slot := 0; slot < len(data)/sizeG3RawHalf; slot++ {
		sector0, ok := allMainSectors(data, slot)
		if !ok {
			continue
		}
		d := data[sector0:]
		switch binary.LittleEndian.Uint32(d[0xAC:]) {
		case 1:
			return kindFRLG, true
		case 0:
			return kindRS, true
		}
		for _, b := range d[0x890:0xF2C] {
			if b != 0 {
				return kindE, true
			}
		}
		return kindRS, true
	}
	return 0, false
}

// Save3 is a Ruby/Sapphire/Emerald/FireRed/LeafGreen save.
type Save3 struct {
	data     []byte
	footer   []byte
	kind     gen3Kind
	game     Game
	slot     int
	small    []byte
	large    []byte
	storage  []byte
	jp       bool
	personal *PersonalTable
	lay      sav3Layout
}

func openSave3(data []byte, kind gen3Kind) (*Save3, error) {
	s := &Save3{data: data, kind: kind, lay: sav3Layouts[kind]}
	s.small = make([]byte, sectorUsed)
	s.large = make([]byte, 4*sectorUsed)
	s.storage = make([]byte, 9*sectorUsed)
	s.slot = s.activeSlot()
	s.readSectors()
	s.jp = binary.LittleEndian.Uint16(s.small[6:]) == 0
	switch kind {
	case kindRS:
		s.game, s.personal = RubySapphire, personalTable("rs", 3)
	case kindE:
		s.game, s.personal = Emerald, personalTable("e", 3)
	default:
		s.game, s.personal = FireRed, personalTable("fr", 3)
	}
	return s, nil
}

func (s *Save3) misconfigured() bool { return len(s.data) < sizeG3Raw }

func (s *Save3) activeSlot() int {
	if len(s.data) == sizeG3RawHalf {
		return 0
	}
	o0, v0 := allMainSectors(s.data, 0)
	o1, v1 := allMainSectors(s.data, 1)
	if !v0 {
		if v1 {
			return 1
		}
		return 0
	}
	if !v1 {
		return 0
	}
	c1 := binary.LittleEndian.Uint32(s.data[o0+0xFFC:])
	c2 := binary.LittleEndian.Uint32(s.data[o1+0xFFC:])
	switch {
	case c1 == 0xFFFFFFFF && c2 != 0xFFFFFFFE:
		return 1
	case c2 == 0xFFFFFFFF && c1 != 0xFFFFFFFE:
		return 0
	case c1 < c2:
		return 1
	}
	return 0
}

func (s *Save3) chunk(id int) []byte {
	switch {
	case id >= 5:
		return s.storage[(id-5)*sectorUsed : (id-4)*sectorUsed]
	case id >= 1:
		return s.large[(id-1)*sectorUsed : id*sectorUsed]
	}
	return s.small
}

func (s *Save3) readSectors() {
	start := s.slot * mainSize
	for ofs := start; ofs < start+mainSize; ofs += sectorSize {
		copy(s.chunk(sectorID(s.data, ofs)), s.data[ofs:ofs+sectorUsed])
	}
}

func (s *Save3) writeSectors() {
	start := s.slot * mainSize
	for ofs := start; ofs < start+mainSize; ofs += sectorSize {
		copy(s.data[ofs:ofs+sectorUsed], s.chunk(sectorID(s.data, ofs)))
	}
}

func checkSum32(b []byte) uint16 {
	var sum uint32
	for i := 0; i+4 <= len(b); i += 4 {
		sum += binary.LittleEndian.Uint32(b[i:])
	}
	return uint16(sum + sum>>16)
}

func sectorUninitialized(sector []byte) bool {
	for _, b := range sector {
		if b != 0 && b != 0xFF {
			return false
		}
	}
	return true
}

// ChecksumsValid verifies every main sector of the active slot and the
// Hall of Fame / extra sectors.
func (s *Save3) ChecksumsValid() bool {
	base := s.slot * mainSize
	for i := 0; i < mainSectors; i++ {
		sec := s.data[base+i*sectorSize : base+(i+1)*sectorSize]
		if checkSum32(sec[:sectorUsed]) != binary.LittleEndian.Uint16(sec[0xFF6:]) {
			return false
		}
	}
	if s.misconfigured() {
		return true
	}
	for i := 0; i < extraSectors; i++ {
		sec := s.data[extraSectorOfs+i*sectorSize : extraSectorOfs+(i+1)*sectorSize]
		if sectorUninitialized(sec) {
			continue
		}
		if checkSum32(sec[:sectorUsed]) != binary.LittleEndian.Uint16(sec[0xFF4:]) {
			return false
		}
	}
	return true
}

func (s *Save3) setChecksums() {
	base := s.slot * mainSize
	for i := 0; i < mainSectors; i++ {
		sec := s.data[base+i*sectorSize : base+(i+1)*sectorSize]
		binary.LittleEndian.PutUint16(sec[0xFF6:], checkSum32(sec[:sectorUsed]))
	}
	if s.misconfigured() {
		return
	}
	for i := 0; i < extraSectors; i++ {
		sec := s.data[extraSectorOfs+i*sectorSize : extraSectorOfs+(i+1)*sectorSize]
		if sectorUninitialized(sec) {
			continue
		}
		binary.LittleEndian.PutUint16(sec[0xFF4:], checkSum32(sec[:sectorUsed]))
	}
}

func (s *Save3) Bytes() ([]byte, error) {
	s.writeSectors()
	s.setChecksums()
	return append(append([]byte(nil), s.data...), s.footer...), nil
}

func (s *Save3) Clone() (Save, error) {
	raw, _ := s.Bytes()
	c, err := Open(raw, OpenOptions{AllowBadChecksums: true})
	if err != nil {
		return nil, err
	}
	_ = c.SetGame(s.game)
	return c, nil
}

func (s *Save3) Game() Game { return s.game }

// SetGame selects the exact game of the save's family. FireRed and LeafGreen
// switch the species data used for new Pokémon.
func (s *Save3) SetGame(g Game) error {
	var ok bool
	switch s.kind {
	case kindRS:
		ok = g == Ruby || g == Sapphire || g == RubySapphire
	case kindE:
		ok = g == Emerald
	default:
		ok = g == FireRed || g == LeafGreen || g == FireRedLeafGreen
	}
	if err := require(ok, "Selected game does not match the detected save format."); err != nil {
		return err
	}
	s.game = g
	if s.kind == kindFRLG {
		if g == LeafGreen {
			s.personal = personalTable("lg", 3)
		} else {
			s.personal = personalTable("fr", 3)
		}
	}
	return nil
}

func (s *Save3) Generation() int { return 3 }
func (s *Save3) Japanese() bool  { return s.jp }

func (s *Save3) Language() Language {
	if s.jp {
		return LanguageJapanese
	}
	return LanguageEnglish
}

func (s *Save3) securityKey() uint32 {
	if s.lay.keyOffset < 0 {
		return 0
	}
	return binary.LittleEndian.Uint32(s.small[s.lay.keyOffset:])
}

// Trainer ----------------------------------------------------------------

func (s *Save3) MaxTrainerLength() int  { return 7 }
func (s *Save3) MaxNicknameLength() int { return 10 }

func (s *Save3) otLen() int {
	if s.jp {
		return 5
	}
	return 7
}

func (s *Save3) OT() string { return decodeG3(s.small[:s.otLen()], s.Language()) }

func (s *Save3) SetOT(name string) error {
	n := s.otLen()
	if err := require(len([]rune(name)) > 0 && len([]rune(name)) <= n, "Trainer/rival name is too long for this game."); err != nil {
		return err
	}
	encodeG3(s.small[:n], name, n, s.Language(), clearNone)
	return nil
}

func (s *Save3) RivalName() (string, bool) {
	if s.lay.rivalOffset < 0 {
		return "", false
	}
	return decodeG3(s.large[s.lay.rivalOffset:s.lay.rivalOffset+8], s.Language()), true
}

func (s *Save3) SetRivalName(name string) error {
	if s.lay.rivalOffset < 0 {
		return fail("Unsupported trainer field.")
	}
	if err := require(len([]rune(name)) > 0 && len([]rune(name)) <= 7, "Trainer/rival name is too long for this game."); err != nil {
		return err
	}
	encodeG3(s.large[s.lay.rivalOffset:s.lay.rivalOffset+8], name, 7, s.Language(), clearZero)
	return nil
}

func (s *Save3) TID() int { return le16(s.small[0xA:]) }
func (s *Save3) SetTID(v int) {
	putLE16(s.small[0xA:], v)
}
func (s *Save3) SID() int { return le16(s.small[0xC:]) }
func (s *Save3) SetSID(v int) {
	putLE16(s.small[0xC:], v)
}
func (s *Save3) Gender() int     { return int(s.small[8]) }
func (s *Save3) SetGender(v int) { s.small[8] = byte(v) }
func (s *Save3) MaxMoney() int   { return 999999 }

func (s *Save3) Money() uint32 {
	return le32(s.large[s.lay.money:]) ^ s.securityKey()
}

func (s *Save3) SetMoney(v uint32) error {
	if err := require(v <= 999999, "Money is out of range."); err != nil {
		return err
	}
	putLE32(s.large[s.lay.money:], v^s.securityKey())
	return nil
}

func (s *Save3) Coins() uint32 {
	return uint32(uint16(le16(s.large[s.lay.coin:])) ^ uint16(s.securityKey()))
}

func (s *Save3) SetCoins(v uint32) error {
	if err := require(v <= 9999, "Coins are out of range."); err != nil {
		return err
	}
	putLE16(s.large[s.lay.coin:], int(uint16(v)^uint16(s.securityKey())))
	return nil
}

func (s *Save3) eventFlag(n int) bool {
	return getFlag(s.large, s.lay.eventFlag+n>>3, n&7)
}

func (s *Save3) setEventFlag(n int, v bool) {
	setFlag(s.large, s.lay.eventFlag+n>>3, n&7, v)
}

// Badges returns the eight gym badges as bit flags.
func (s *Save3) Badges() int {
	v := 0
	for i := 0; i < 8; i++ {
		if s.eventFlag(s.lay.badgeFlag + i) {
			v |= 1 << i
		}
	}
	return v
}

func (s *Save3) SetBadges(v int) {
	for i := 0; i < 8; i++ {
		s.setEventFlag(s.lay.badgeFlag+i, v&(1<<i) != 0)
	}
}

func (s *Save3) MaxSpecies() int { return maxSpecies3 }
func (s *Save3) MaxMove() int    { return maxMove3 }
func (s *Save3) MaxIV() int      { return 31 }
func (s *Save3) MaxEV() int      { return 255 }

func (s *Save3) HeldItems() []uint16 { return HeldItemIDs(Emerald) }

func (s *Save3) Personal() *PersonalTable { return s.personal }

func (s *Save3) BlankPokemon() *Pokemon { return NewPokemon(3, false) }

// Storage -------------------------------------------------------------------

func (s *Save3) PartyCount() int { return int(s.large[s.lay.partyCount]) }

func (s *Save3) setPartyCount(n int) { s.large[s.lay.partyCount] = byte(n) }

func (s *Save3) partySlot(i int) []byte {
	return s.large[s.lay.partyBuf+i*size3Party : s.lay.partyBuf+(i+1)*size3Party]
}

func (s *Save3) Party(i int) (*Pokemon, error) {
	if i < 0 || i >= s.PartyCount() {
		return nil, fail("Select an occupied Pokémon slot.")
	}
	return pokemon3FromBytes(s.partySlot(i)), nil
}

// IsMail reports whether a generation 3 item id is a Mail.
func IsMail(item int) bool { return item >= 121 && item <= 132 }

func (s *Save3) SetParty(i int, pk *Pokemon) error {
	if err := checkFormat(pk, 3, false); err != nil {
		return err
	}
	if i < 0 || i > 5 || i > s.PartyCount() {
		return fail("Select an occupied Pokémon slot.")
	}
	pk = pk.Clone()
	if !IsMail(pk.HeldItem()) {
		pk.setHeldMailID(-1)
	}
	if !pk.PartyStatsPresent() {
		pk.ResetPartyStats()
	}
	pk.RefreshChecksum()
	copy(s.partySlot(i), pk.partyBody3())
	if i >= s.PartyCount() {
		s.setPartyCount(i + 1)
	}
	return nil
}

func (s *Save3) DeleteParty(i int) error {
	n := s.PartyCount()
	if i < 0 || i >= n {
		return nil
	}
	for j := i + 1; j < n; j++ {
		copy(s.partySlot(j-1), s.partySlot(j))
	}
	clear(s.partySlot(n - 1))
	s.setPartyCount(n - 1)
	return nil
}

const (
	boxes3     = 14
	boxSlots3  = 30
	boxNameLen = 9
)

func (s *Save3) BoxCount() int     { return boxes3 }
func (s *Save3) BoxSlotCount() int { return boxSlots3 }
func (s *Save3) CurrentBox() int   { return int(s.storage[0]) }

func (s *Save3) boxSlot(box, slot int) []byte {
	o := 4 + size3Stored*(box*boxSlots3+slot)
	return s.storage[o : o+size3Stored]
}

func (s *Save3) boxOK(box, slot int) error {
	if box < 0 || box >= boxes3 {
		return fail("Invalid box.")
	}
	if slot < 0 || slot >= boxSlots3 {
		return fail("Select an occupied Pokémon slot.")
	}
	return nil
}

func present3(b []byte) bool { return b[0x13]&0xFB == 2 }

func (s *Save3) Box(box, slot int) (*Pokemon, error) {
	if err := s.boxOK(box, slot); err != nil {
		return nil, err
	}
	raw := s.boxSlot(box, slot)
	if !present3(raw) {
		return nil, nil
	}
	return pokemon3FromBytes(raw), nil
}

func (s *Save3) SetBox(box, slot int, pk *Pokemon) error {
	if err := checkFormat(pk, 3, false); err != nil {
		return err
	}
	if err := s.boxOK(box, slot); err != nil {
		return err
	}
	pk = pk.Clone()
	pk.RefreshChecksum()
	copy(s.boxSlot(box, slot), pk.storedBody3())
	return nil
}

func (s *Save3) DeleteBox(box, slot int) error {
	if err := s.boxOK(box, slot); err != nil {
		return err
	}
	clear(s.boxSlot(box, slot))
	return nil
}

func (s *Save3) FirstFreeBoxSlot(box int) int {
	if box < 0 || box >= boxes3 {
		return -1
	}
	for i := 0; i < boxSlots3; i++ {
		if !present3(s.boxSlot(box, i)) {
			return i
		}
	}
	return -1
}

// BoxName returns the name of a box.
func (s *Save3) BoxName(box int) string {
	o := 4 + size3Stored*boxes3*boxSlots3 + box*boxNameLen
	return decodeG3(s.storage[o:o+boxNameLen], s.Language())
}

// Pokédex ---------------------------------------------------------------------

const (
	dexBase3   = 0x18
	dexCaught3 = dexBase3 + 0x10
	dexSeen3   = dexBase3 + 0x44
)

func (s *Save3) Caught(species int) bool {
	if species < 1 || species > maxSpecies3 {
		return false
	}
	bit := species - 1
	return getFlag(s.small, dexCaught3+bit>>3, bit&7)
}

func (s *Save3) Seen(species int) bool {
	if species < 1 || species > maxSpecies3 {
		return false
	}
	bit := species - 1
	return getFlag(s.small, dexSeen3+bit>>3, bit&7)
}

func (s *Save3) SetCaught(species int, v bool) {
	if species < 1 || species > maxSpecies3 {
		return
	}
	bit := species - 1
	setFlag(s.small, dexCaught3+bit>>3, bit&7, v)
}

// SetSeen updates the three mirrored seen bitmaps the games keep.
func (s *Save3) SetSeen(species int, v bool) {
	if species < 1 || species > maxSpecies3 {
		return
	}
	bit := species - 1
	setFlag(s.small, dexSeen3+bit>>3, bit&7, v)
	setFlag(s.large, s.lay.seen2+bit>>3, bit&7, v)
	setFlag(s.large, s.lay.seen3+bit>>3, bit&7, v)
}

// Inventory ----------------------------------------------------------------------

func (s *Save3) inventory() []byte { return s.large[s.lay.inventory:] }

func (s *Save3) defs() []pouchDef { return gen3Defs(s.kind, s.securityKey()) }

func (s *Save3) Pockets() []Pocket { return pocketsOf(s.defs(), s.inventory()) }

func (s *Save3) SetPocket(i int, entries []ItemStack) error {
	return setPocketIn(s.defs(), s.inventory(), i, entries)
}
