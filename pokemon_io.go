package pokeedit

import (
	"encoding/binary"
	"encoding/hex"
)

// Generation 3 block order for each of the 24 personality value remainders.
var blockPosition = [24][4]byte{
	{0, 1, 2, 3}, {0, 1, 3, 2}, {0, 2, 1, 3}, {0, 3, 1, 2}, {0, 2, 3, 1}, {0, 3, 2, 1},
	{1, 0, 2, 3}, {1, 0, 3, 2}, {2, 0, 1, 3}, {3, 0, 1, 2}, {2, 0, 3, 1}, {3, 0, 2, 1},
	{1, 2, 0, 3}, {1, 3, 0, 2}, {2, 1, 0, 3}, {3, 1, 0, 2}, {2, 3, 0, 1}, {3, 2, 0, 1},
	{1, 2, 3, 0}, {1, 3, 2, 0}, {2, 1, 3, 0}, {3, 1, 2, 0}, {2, 3, 1, 0}, {3, 2, 1, 0},
}

var blockPositionInvert = [24]byte{
	0, 1, 2, 4, 3, 5, 6, 7, 12, 18, 13, 19, 8, 10, 14, 20, 16, 22, 9, 11, 15, 21, 17, 23,
}

func shuffle3(data []byte, sv byte) {
	if sv == 0 {
		return
	}
	src := append([]byte(nil), data...)
	for block := 0; block < 4; block++ {
		ofs := int(blockPosition[sv][block])
		copy(data[block*12:(block+1)*12], src[ofs*12:(ofs+1)*12])
	}
}

func cryptArray3(data []byte, seed uint32) {
	for i := 0; i+4 <= len(data); i += 4 {
		binary.LittleEndian.PutUint32(data[i:], binary.LittleEndian.Uint32(data[i:])^seed)
	}
}

// decrypt3 decrypts an 80-byte generation 3 record in place.
func decrypt3(data []byte) {
	pid := binary.LittleEndian.Uint32(data)
	oid := binary.LittleEndian.Uint32(data[4:])
	body := data[32:80]
	cryptArray3(body, pid^oid)
	shuffle3(body, byte(pid%24))
}

// encrypt3 encrypts an 80-byte generation 3 record in place.
func encrypt3(data []byte) {
	pid := binary.LittleEndian.Uint32(data)
	oid := binary.LittleEndian.Uint32(data[4:])
	body := data[32:80]
	shuffle3(body, blockPositionInvert[pid%24])
	cryptArray3(body, pid^oid)
}

func isEncrypted3(data []byte) bool {
	return add16(data[32:80]) != uint16(binary.LittleEndian.Uint16(data[0x1C:]))
}

// marker is the species byte used by generation 1/2 list headers.
func (p *Pokemon) marker() byte {
	s := p.speciesInternal()
	if s == 0 {
		return slotEmpty
	}
	if p.format == 2 && p.egg {
		return slotEgg
	}
	return byte(s)
}

// Bytes returns the pokegarden/PKHeX "decrypted party" encoding of the
// Pokémon: a single-entry list for generations 1/2 (69/73 bytes, 59/63 for
// Japanese) and the 100-byte decrypted record for generation 3. ParsePokemon
// reads it back.
func (p *Pokemon) Bytes() []byte {
	if p.format == 3 {
		c := p.Clone()
		c.RefreshChecksum()
		return c.data
	}
	n := len(p.ot)
	out := make([]byte, 1+2+len(p.data)+2*n)
	m := p.marker()
	if m != slotEmpty && m != 0 {
		out[0] = 1
	}
	out[1] = m
	out[2] = slotEmpty
	copy(out[3:], p.data)
	copy(out[3+len(p.data):], p.ot)
	copy(out[3+len(p.data)+n:], p.nick)
	return out
}

// ParsePokemon decodes the encoding produced by Bytes. Generation 3 records
// are also accepted in their 80-byte stored or encrypted forms.
func ParsePokemon(b []byte) (*Pokemon, error) {
	switch len(b) {
	case size1UList, size1JList, size2UList, size2JList:
		format, body, n := 1, size1Party, strLenIntl
		if len(b) == size2UList || len(b) == size2JList {
			format, body = 2, size2Party
		}
		if len(b) == size1JList || len(b) == size2JList {
			n = strLenJP
		}
		p := &Pokemon{format: format, data: append([]byte(nil), b[3:3+body]...)}
		p.ot = append([]byte(nil), b[3+body:3+body+n]...)
		p.nick = append([]byte(nil), b[3+body+n:3+body+2*n]...)
		p.egg = format == 2 && b[1] == slotEgg
		return p, nil
	case size3Stored, size3Party:
		return pokemon3FromBytes(b), nil
	}
	return nil, fail("Invalid Pokémon record.")
}

func pokemon3FromBytes(b []byte) *Pokemon {
	data := make([]byte, size3Party)
	copy(data, b)
	if isEncrypted3(data) {
		decrypt3(data[:size3Stored])
	}
	return &Pokemon{format: 3, data: data}
}

// NewPokemonFromParts rebuilds a generation 1 Pokémon from the legacy
// core/ot/nickname hex encoding that early pokegarden versions stored.
func NewPokemonFromParts(coreHex, otHex, nickHex string) (*Pokemon, error) {
	core, err1 := hex.DecodeString(coreHex)
	ot, err2 := hex.DecodeString(otHex)
	nick, err3 := hex.DecodeString(nickHex)
	if err1 != nil || err2 != nil || err3 != nil || len(ot) != len(nick) ||
		(len(ot) != strLenIntl && len(ot) != strLenJP) || len(core) > size1Party {
		return nil, fail("Invalid Pokémon record.")
	}
	p := &Pokemon{format: 1, data: make([]byte, size1Party), ot: ot, nick: nick}
	copy(p.data, core)
	return p, nil
}

// LegacyParts returns the generation 1 core/ot/nickname byte slices used by
// the legacy encoding (core is the 33-byte stored form).
func (p *Pokemon) LegacyParts() (core, ot, nick []byte) {
	if p.format != 1 {
		return nil, nil, nil
	}
	return append([]byte(nil), p.data[:size1Stored]...), append([]byte(nil), p.ot...), append([]byte(nil), p.nick...)
}

// storedBody returns the box-storage form of a generation 3 Pokémon (80
// encrypted bytes).
func (p *Pokemon) storedBody3() []byte {
	out := append([]byte(nil), p.data[:size3Stored]...)
	encrypt3(out)
	return out
}

// partyBody3 returns the party-storage form of a generation 3 Pokémon (80
// encrypted bytes followed by the 20 plaintext party bytes).
func (p *Pokemon) partyBody3() []byte {
	out := append([]byte(nil), p.data...)
	encrypt3(out[:size3Stored])
	return out
}

// ---------------------------------------------------------------------------
// Conversion between generations 1 and 2

var teruSamaItem = map[byte]byte{
	0x19: 0x92, 0x2D: 0x53, 0x32: 0xAE,
	0x5A: 0xAD, 0x64: 0xAD, 0x78: 0xAD, 0x87: 0xAD, 0xBE: 0xAD, 0xC3: 0xAD, 0xDC: 0xAD, 0xFA: 0xAD, 0xFF: 0xAD,
}

// gen2ItemFromCatchRate maps a generation 1 catch rate byte to the held item a
// generation 2 game would read from it.
func gen2ItemFromCatchRate(rate byte) byte {
	if isCatchRateHeldItem(rate) {
		return rate
	}
	if v, ok := teruSamaItem[rate]; ok {
		return v
	}
	return rate
}

// toGen2 converts a generation 1 Pokémon to the generation 2 layout, keeping
// current HP (clamped to the recalculated maximum).
func (p *Pokemon) toGen2() *Pokemon {
	pk2 := NewPokemon(2, p.Japanese())
	pk2.SetSpecies(p.Species())
	copy(pk2.data[1:1+0x1A], p.data[7:7+0x1A])
	copy(pk2.ot, p.ot)
	copy(pk2.nick, p.nick)
	pk2.SetHeldItem(int(gen2ItemFromCatchRate(p.data[7])))
	pk2.SetFriendship(int(personalTable("c", 2).Get(pk2.Species()).BaseFriendship))
	hp := p.HP()
	pk2.SetStatLevel(p.Level())
	if p.PartyStatsPresent() {
		pk2.ResetPartyStats()
		if hp < pk2.MaxHP() {
			pk2.SetHP(hp)
		}
	}
	return pk2
}

// toGen1 converts a generation 2 Pokémon to the generation 1 layout.
func (p *Pokemon) toGen1() *Pokemon {
	pk1 := NewPokemon(1, p.Japanese())
	copy(pk1.data[7:7+0x1A], p.data[1:1+0x1A])
	pk1.SetSpecies(p.Species())
	if p.StatLevel() == 0 {
		pk1.SetHP(gbStat(p.personal().HP, p.IV(StatAttack), p.EV(StatAttack), p.StatLevel()))
		pk1.SetStatLevel(p.Level())
	} else {
		pk1.SetHP(p.HP())
		pk1.SetStatLevel(p.StatLevel())
	}
	copy(pk1.ot, p.ot)
	copy(pk1.nick, p.nick)
	// Moves a generation 1 game does not know are removed.
	invalid, moves := 0, p.Moves()
	for i, m := range moves {
		if m > maxMove1 {
			moves[i] = 0
			invalid++
		}
	}
	if invalid == 4 {
		moves[0] = 1
	}
	var kept []int
	for _, m := range moves {
		if m != 0 {
			kept = append(kept, m)
		}
	}
	for i := 0; i < 4; i++ {
		m := 0
		if i < len(kept) {
			m = kept[i]
		}
		pk1.SetMove(i, m)
	}
	return pk1
}

// ConvertFor converts a Pokémon so it can be stored in a save of the given
// generation, applying the same compatibility rules as the pokegarden engine:
// GB/GBC and GBA never mix, species and moves must exist in the destination,
// and Japanese/international records cannot be mixed.
func (p *Pokemon) ConvertFor(generation int, japanese bool) (*Pokemon, error) {
	if p.Species() > MaxSpecies(generation) {
		return nil, fail("Species is not supported by the destination game.")
	}
	if (p.format <= 2) != (generation <= 2) {
		return nil, fail("GB/GBC and GBA formats have no compatible transfer route.")
	}
	if p.format <= 2 && p.Japanese() != japanese {
		return nil, fail("Japanese/international/Korean GB records cannot be mixed.")
	}
	for _, m := range p.Moves() {
		if m > MaxMove(generation) {
			return nil, fail("A move is unsupported by the destination game.")
		}
	}
	if generation == 1 && (p.IsEgg() || (p.format == 2 && p.HeldItem() != 0)) {
		return nil, fail("Remove the held item before returning to Generation I.")
	}
	switch {
	case p.format == generation:
		return p.Clone(), nil
	case p.format == 1 && generation == 2:
		return p.toGen2(), nil
	case p.format == 2 && generation == 1:
		return p.toGen1(), nil
	}
	return nil, fail("No compatible transfer route to this game.")
}
