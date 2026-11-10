package pokeedit

// Save is an opened battery save. All mutations happen in memory; Bytes
// finalizes checksums and returns the file contents to write.
type Save interface {
	// Game is the detected or selected game (or game family).
	Game() Game
	// SetGame selects the exact game of a paired family (for example Red
	// versus Blue). The game must belong to the detected save family.
	SetGame(Game) error
	Generation() int
	Language() Language
	Japanese() bool

	// ChecksumsValid reports whether every checksum the library verifies matches.
	ChecksumsValid() bool
	// Bytes recomputes checksums and returns the complete save file, including
	// any emulator footer that was present on load.
	Bytes() ([]byte, error)
	// Clone returns an independent copy.
	Clone() (Save, error)

	// Trainer.
	OT() string
	SetOT(string) error
	TID() int
	SetTID(int)
	SID() int
	SetSID(int)
	Gender() int
	SetGender(int)
	Money() uint32
	SetMoney(uint32) error
	Coins() uint32
	SetCoins(uint32) error
	Badges() int
	SetBadges(int)
	// RivalName reports ok=false when the game exposes no editable rival name.
	RivalName() (name string, ok bool)
	SetRivalName(string) error

	// Limits and tables.
	MaxSpecies() int
	MaxMove() int
	MaxIV() int
	MaxEV() int
	MaxMoney() int
	MaxNicknameLength() int
	MaxTrainerLength() int
	Personal() *PersonalTable
	// HeldItems lists the item ids a Pokémon can hold in this game.
	HeldItems() []uint16

	// Storage.
	PartyCount() int
	Party(i int) (*Pokemon, error)
	SetParty(i int, pk *Pokemon) error
	DeleteParty(i int) error
	BoxCount() int
	BoxSlotCount() int
	CurrentBox() int
	// Box returns nil, nil for an empty slot.
	Box(box, slot int) (*Pokemon, error)
	SetBox(box, slot int, pk *Pokemon) error
	DeleteBox(box, slot int) error
	// FirstFreeBoxSlot returns the first empty slot of a box, or -1.
	FirstFreeBoxSlot(box int) int
	// BlankPokemon returns an empty Pokémon of this save's format.
	BlankPokemon() *Pokemon

	// Pokédex.
	Seen(species int) bool
	Caught(species int) bool
	SetSeen(species int, seen bool)
	SetCaught(species int, caught bool)

	// Inventory.
	Pockets() []Pocket
	SetPocket(index int, entries []ItemStack) error
}

// OpenOptions control how a save is interpreted.
type OpenOptions struct {
	// Game optionally selects the exact game of a paired family (for example
	// "BU" for a Red/Blue save). It must match the detected save format.
	Game Game
	// AllowBadChecksums opens saves whose checksums do not match.
	AllowBadChecksums bool
}

const (
	sizeG1Raw     = 0x8000
	sizeG2RawU    = 0x8000
	sizeG2RawJ    = 0x10000
	sizeG3Raw     = 0x20000
	sizeG3RawHalf = 0x10000
)

// splitFooter separates an emulator footer (RTC data and the like) from the
// battery save proper using PKHeX's size heuristics.
func splitFooter(data []byte) (body, footer []byte) {
	n := len(data)
	f := n & 0x3F
	plausible := f == 7 || (f&1 == 0 && f >= 0x0C && f <= 0x30)
	if plausible {
		switch n - f {
		case sizeG3Raw, sizeG2RawJ, sizeG1Raw:
			return data[:n-f], data[n-f:]
		}
	}
	return data, nil
}

// Open detects the save type of raw battery save bytes and parses it.
func Open(raw []byte, opts OpenOptions) (Save, error) {
	body, footer := splitFooter(raw)
	data := append([]byte(nil), body...)
	foot := append([]byte(nil), footer...)
	var (
		sav Save
		err error
	)
	switch len(data) {
	case sizeG1Raw:
		if g1, ok := detectGen1(data); ok {
			sav, err = openSave1(data, g1)
			break
		}
		if g2, ok := detectGen2(data); ok {
			sav, err = openSave2(data, g2)
			break
		}
	case sizeG2RawJ:
		if g2, ok := detectGen2(data); ok {
			sav, err = openSave2(data, g2)
			break
		}
		fallthrough
	case sizeG3Raw:
		if kind, ok := detectGen3(data); ok {
			sav, err = openSave3(data, kind)
		}
	}
	if err != nil {
		return nil, err
	}
	if sav == nil {
		return nil, fail("Unrecognized battery save. Save states and ROM hacks are unsupported.")
	}
	if !opts.AllowBadChecksums && !sav.ChecksumsValid() {
		return nil, fail("Save checksums are invalid; refusing to edit a damaged save.")
	}
	setFooter(sav, foot)
	if opts.Game != "" {
		if err := sav.SetGame(opts.Game); err != nil {
			return nil, err
		}
	} else if s3, ok := sav.(*Save3); ok && s3.kind == kindFRLG {
		s3.game = FireRedLeafGreen
	}
	return sav, nil
}

func setFooter(s Save, f []byte) {
	switch v := s.(type) {
	case *Save1:
		v.footer = f
	case *Save2:
		v.footer = f
	case *Save3:
		v.footer = f
	}
}

// ---------------------------------------------------------------------------
// Generation 1/2 Pokémon lists

func gbBodySize(format int, party bool) int {
	if format == 1 {
		if party {
			return size1Party
		}
		return size1Stored
	}
	if party {
		return size2Party
	}
	return size2Stored
}

func gbStrLen(jp bool) int {
	if jp {
		return strLenJP
	}
	return strLenIntl
}

func gbListLength(format, capacity int, jp, party bool) int {
	return 1 + (capacity + 1) + gbBodySize(format, party)*capacity + gbStrLen(jp)*capacity*2
}

// unpackGBList reads a packed party/box list into Pokémon.
func unpackGBList(src []byte, format, capacity int, jp, party bool) []*Pokemon {
	body := gbBodySize(format, party)
	sl := gbStrLen(jp)
	ofsBody := 1 + capacity + 1
	ofsStr1 := ofsBody + capacity*body
	ofsStr2 := ofsStr1 + capacity*sl
	count := int(src[0])
	if count > capacity {
		count = capacity
	}
	out := make([]*Pokemon, 0, count)
	full := size1Party
	if format == 2 {
		full = size2Party
	}
	for i := 0; i < count; i++ {
		pk := &Pokemon{format: format, data: make([]byte, full)}
		copy(pk.data, src[ofsBody+i*body:ofsBody+(i+1)*body])
		pk.ot = append([]byte(nil), src[ofsStr1+i*sl:ofsStr1+(i+1)*sl]...)
		pk.nick = append([]byte(nil), src[ofsStr2+i*sl:ofsStr2+(i+1)*sl]...)
		pk.egg = format == 2 && src[1+i] == slotEgg
		out = append(out, pk)
	}
	return out
}

// mergeGBList writes Pokémon into a packed list. It returns false (leaving
// dest untouched) for an empty list over an uninitialized destination.
func mergeGBList(mons []*Pokemon, dest []byte, format, capacity int, jp, party, destInitialized bool, prev int) bool {
	allZero := true
	for _, b := range dest {
		if b != 0 {
			allZero = false
			break
		}
	}
	if len(mons) == 0 && (!destInitialized || allZero) {
		return false
	}
	body := gbBodySize(format, party)
	sl := gbStrLen(jp)
	ofsBody := 1 + capacity + 1
	ofsStr1 := ofsBody + capacity*body
	ofsStr2 := ofsStr1 + capacity*sl
	dest[0] = byte(len(mons))
	for i := 0; i < capacity; i++ {
		if i >= len(mons) {
			// Unused slots keep whatever bytes the game left there (it never
			// clears them either) unless a Pokémon was just removed from them.
			dest[1+i] = slotEmpty
			if i < prev {
				clear(dest[ofsBody+i*body : ofsBody+(i+1)*body])
				clear(dest[ofsStr1+i*sl : ofsStr1+(i+1)*sl])
				clear(dest[ofsStr2+i*sl : ofsStr2+(i+1)*sl])
			}
			continue
		}
		pk := mons[i]
		dest[1+i] = pk.marker()
		copy(dest[ofsBody+i*body:ofsBody+(i+1)*body], pk.data[:body])
		copy(dest[ofsStr1+i*sl:ofsStr1+(i+1)*sl], pk.ot)
		copy(dest[ofsStr2+i*sl:ofsStr2+(i+1)*sl], pk.nick)
	}
	dest[1+capacity] = slotEmpty
	return true
}

// initEmptyGBList writes the header of an empty list (count 0, terminator).
func initEmptyGBList(dest []byte, capacity int) {
	dest[0] = 0
	for i := 0; i <= capacity; i++ {
		dest[1+i] = slotEmpty
	}
}

// listValid reports whether a packed list header is plausible.
func listValid(data []byte, offset, maxCount int) bool {
	if offset+1 >= len(data) {
		return false
	}
	count := int(data[offset])
	return count <= maxCount && offset+1+count < len(data) && data[offset+1+count] == 0xFF
}

// ---------------------------------------------------------------------------
// Shared helpers

func getFlag(data []byte, offset, bit int) bool { return data[offset]>>uint(bit)&1 == 1 }

func setFlag(data []byte, offset, bit int, v bool) {
	if v {
		data[offset] |= 1 << uint(bit)
	} else {
		data[offset] &^= 1 << uint(bit)
	}
}

// bcdRead decodes n big-endian BCD bytes.
func bcdRead(b []byte) uint32 {
	var v uint32
	for _, x := range b {
		v = v*100 + uint32(x>>4)*10 + uint32(x&0xF)
	}
	return v
}

// bcdWrite encodes v as n big-endian BCD bytes.
func bcdWrite(b []byte, v uint32) {
	for i := len(b) - 1; i >= 0; i-- {
		d := v % 100
		b[i] = byte(d/10)<<4 | byte(d%10)
		v /= 100
	}
}
