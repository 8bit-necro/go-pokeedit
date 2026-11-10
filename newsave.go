package pokeedit

import "encoding/binary"

// NewSaveOptions configure NewSave.
type NewSaveOptions struct {
	OT       string // default "GARDEN"
	TID      int    // default 1234
	Japanese bool   // generation 1/2 Japanese layout (not supported for generation 3)
}

// NewSave builds a blank, valid save for a game: no Pokémon, empty bag. It is
// useful for tests and for starting an empty collection. Use Party/Box
// helpers such as AddPokemon to fill it.
func NewSave(game Game, opts NewSaveOptions) (Save, error) {
	if !game.Valid() {
		return nil, fail("Unsupported GB/GBC/GBA game.")
	}
	if opts.OT == "" {
		opts.OT = "GARDEN"
	}
	if opts.TID == 0 {
		opts.TID = 1234
	}
	var (
		sav Save
		err error
	)
	switch game.Generation() {
	case 1:
		sav, err = newSave1(game, opts)
	case 2:
		sav, err = newSave2(game, opts)
	default:
		sav, err = newSave3(game, opts)
	}
	if err != nil {
		return nil, err
	}
	if err := sav.SetGame(game); err != nil {
		return nil, err
	}
	if err := sav.SetOT(opts.OT); err != nil {
		return nil, err
	}
	sav.SetTID(opts.TID)
	return sav, nil
}

func newSave1(game Game, opts NewSaveOptions) (Save, error) {
	data := make([]byte, sizeG1Raw)
	o := &sav1Intl
	if opts.Japanese {
		o = &sav1Japan
	}
	s := &Save1{data: data, jp: opts.Japanese, o: o, family: RedBlue}
	if game == Yellow {
		s.family = Yellow
		data[o.starter] = 0x54
	}
	s.game = s.family
	initEmptyGBList(data[o.party:], 6)
	initEmptyGBList(data[o.currentBox:], s.boxCapacity())
	s.load()
	return s, nil
}

func newSave2(game Game, opts NewSaveOptions) (Save, error) {
	data := make([]byte, sizeG2RawU)
	if opts.Japanese {
		data = make([]byte, sizeG2RawJ)
	}
	s := &Save2{data: data, jp: opts.Japanese, crystal: game == Crystal, game: GoldSilver}
	s.o = newSav2Offsets(s.jp, s.crystal)
	initEmptyGBList(data[s.o.party:], 6)
	initEmptyGBList(data[s.o.currentBox:], s.capacity())
	for i := 0; i < s.BoxCount(); i++ {
		initEmptyGBList(data[s.rawBoxOffset(i):], s.capacity())
	}
	s.load()
	return s, nil
}

func newSave3(game Game, opts NewSaveOptions) (Save, error) {
	if opts.Japanese {
		return nil, fail("Japanese generation 3 saves are not supported for new saves.")
	}
	var kind gen3Kind
	switch game {
	case Emerald:
		kind = kindE
	case FireRed, LeafGreen, FireRedLeafGreen:
		kind = kindFRLG
	default:
		kind = kindRS
	}
	data := make([]byte, sizeG3Raw)
	for bank := 0; bank < 2; bank++ {
		for sector := 0; sector < mainSectors; sector++ {
			ofs := bank*mainSize + sector*sectorSize
			binary.LittleEndian.PutUint16(data[ofs+0xFF4:], uint16(sector))
			binary.LittleEndian.PutUint32(data[ofs+0xFF8:], 0x08012025)
			if sector == 0 {
				switch kind {
				case kindE:
					data[ofs+0xAC] = 42
					data[ofs+0x890] = 1
				case kindFRLG:
					data[ofs+0xAC] = 1
				}
			}
		}
	}
	s, err := openSave3(data, kind)
	if err != nil {
		return nil, err
	}
	// Mark the player name as a non-Japanese record (bytes 6–7 non-zero).
	for i := 0; i < 8; i++ {
		s.small[i] = 0xFF
	}
	s.jp = false
	return s, nil
}
