package pokeedit

import "encoding/binary"

// Pocket names, matching PKHeX's inventory types.
const (
	PocketItems    = "Items"
	PocketKeyItems = "KeyItems"
	PocketBalls    = "Balls"
	PocketTMHMs    = "TMHMs"
	PocketBerries  = "Berries"
	PocketPC       = "PCItems"
)

// ItemStack is an item and its quantity in a pocket slot.
type ItemStack struct {
	Slot  int // position in the pocket
	Item  int
	Count int
}

// Pocket is one bag pocket (or the PC item box) of a save.
type Pocket struct {
	Index    int
	Type     string
	Capacity int
	MaxCount int
	// Allowed lists the item ids the pocket accepts.
	Allowed []uint16
	// Entries are the occupied slots in order.
	Entries []ItemStack

	hmMin, hmMax int
}

// MaxFor returns the largest quantity of an item (HMs are limited to one).
func (p *Pocket) MaxFor(item int) int {
	if p.Type == PocketTMHMs && item >= p.hmMin && item <= p.hmMax {
		return 1
	}
	return p.MaxCount
}

type pouchFormat int

const (
	pouchGBList pouchFormat = iota
	pouchGBKeys
	pouchGBTM
	pouchGBA
)

type pouchDef struct {
	typ          string
	format       pouchFormat
	offset       int
	size         int
	maxCount     int
	allowed      []uint16
	key          uint16
	hmMin, hmMax int
}

func (d *pouchDef) slots() int {
	if d.format == pouchGBTM {
		return len(d.allowed)
	}
	return d.size
}

func (d *pouchDef) read(buf []byte) []ItemStack {
	data := buf[d.offset:]
	var out []ItemStack
	switch d.format {
	case pouchGBTM:
		slot := 0
		for i := range d.allowed {
			if data[i] == 0 {
				continue
			}
			out = append(out, ItemStack{Slot: slot, Item: int(d.allowed[i]), Count: int(data[i])})
			slot++
		}
	case pouchGBKeys, pouchGBList:
		n := int(data[0])
		if n > d.size { // uninitialized (0xFF) pouch
			n = 0
		}
		for i := 0; i < n; i++ {
			if d.format == pouchGBKeys {
				out = append(out, ItemStack{Slot: i, Item: int(data[i+1]), Count: 1})
			} else {
				out = append(out, ItemStack{Slot: i, Item: int(data[i*2+1]), Count: int(data[i*2+2])})
			}
		}
		out = nonEmpty(out)
	case pouchGBA:
		for i := 0; i < d.size; i++ {
			item := int(binary.LittleEndian.Uint16(data[i*4:]))
			count := int(binary.LittleEndian.Uint16(data[i*4+2:]) ^ d.key)
			out = append(out, ItemStack{Slot: i, Item: item, Count: count})
		}
		out = nonEmpty(out)
	}
	return out
}

func nonEmpty(in []ItemStack) []ItemStack {
	var out []ItemStack
	for _, s := range in {
		if s.Item != 0 && s.Count > 0 {
			out = append(out, s)
		}
	}
	return out
}

func (d *pouchDef) write(buf []byte, items []ItemStack) {
	data := buf[d.offset:]
	switch d.format {
	case pouchGBTM:
		clear(data[:len(d.allowed)])
		for _, it := range items {
			for i, id := range d.allowed {
				if int(id) == it.Item {
					data[i] = byte(it.Count)
				}
			}
		}
	case pouchGBKeys:
		// Same write pattern as PKHeX: count, every slot, then a terminator
		// (omitted when full — it would land on the next pocket's count).
		data[0] = byte(len(items))
		clear(data[1 : 1+d.size])
		for i, it := range items {
			data[i+1] = byte(it.Item)
		}
		if len(items) < d.size {
			data[1+len(items)] = 0xFF
		}
	case pouchGBList:
		data[0] = byte(len(items))
		clear(data[1 : 1+2*d.size])
		for i, it := range items {
			data[i*2+1] = byte(it.Item)
			data[i*2+2] = byte(it.Count)
		}
		data[1+2*len(items)] = 0xFF
	case pouchGBA:
		for i := 0; i < d.size; i++ {
			var item, count int
			if i < len(items) {
				item, count = items[i].Item, items[i].Count
			}
			binary.LittleEndian.PutUint16(data[i*4:], uint16(item))
			binary.LittleEndian.PutUint16(data[i*4+2:], uint16(count)^d.key)
		}
	}
}

func pocketsOf(defs []pouchDef, buf []byte) []Pocket {
	out := make([]Pocket, len(defs))
	for i := range defs {
		d := &defs[i]
		out[i] = Pocket{
			Index: i, Type: d.typ, Capacity: d.slots(), MaxCount: d.maxCount,
			Allowed: d.allowed, Entries: d.read(buf), hmMin: d.hmMin, hmMax: d.hmMax,
		}
	}
	return out
}

func setPocketIn(defs []pouchDef, buf []byte, index int, entries []ItemStack) error {
	if index < 0 || index >= len(defs) {
		return fail("Invalid inventory pocket.")
	}
	d := &defs[index]
	if len(entries) > d.slots() {
		return fail("Pocket is full.")
	}
	pocket := Pocket{Type: d.typ, MaxCount: d.maxCount, hmMin: d.hmMin, hmMax: d.hmMax}
	items := make([]ItemStack, len(entries))
	for i, e := range entries {
		ok := false
		for _, id := range d.allowed {
			if int(id) == e.Item {
				ok = true
				break
			}
		}
		if !ok {
			return fail("Item is not supported in this game's pocket.")
		}
		if e.Count <= 0 || e.Count > pocket.MaxFor(e.Item) {
			return fail("Invalid item quantity.")
		}
		items[i] = ItemStack{Slot: i, Item: e.Item, Count: e.Count}
	}
	d.write(buf, items)
	// Like PKHeX, normalize the other Game Boy pockets too (list terminators).
	for i := range defs {
		if i != index && defs[i].format != pouchGBA {
			defs[i].write(buf, defs[i].read(buf))
		}
	}
	return nil
}

func concat(lists ...[]uint16) []uint16 {
	var out []uint16
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

// gen2Items returns the allowed items of a generation 2 pocket type.
func gen2Items(crystal bool, typ string) []uint16 {
	pcC := concat(items2General, items2Balls, items2Machine, items2KeyCrystal)
	switch typ {
	case PocketItems:
		return items2General
	case PocketKeyItems:
		if crystal {
			return items2KeyCrystal
		}
		return items2KeyGS
	case PocketTMHMs:
		return items2Machine
	case PocketBalls:
		return items2Balls
	}
	if crystal {
		return pcC
	}
	return pcC[:len(pcC)-4]
}

func gen1Defs(items, pc int) []pouchDef {
	return []pouchDef{
		{typ: PocketItems, format: pouchGBList, offset: items, size: 20, maxCount: 99, allowed: items1General},
		{typ: PocketPC, format: pouchGBList, offset: pc, size: 50, maxCount: 99, allowed: items1General},
	}
}

type gen2PouchOffsets struct{ tm, item, key, ball, pc int }

func gen2Defs(crystal bool, o gen2PouchOffsets) []pouchDef {
	return []pouchDef{
		{typ: PocketTMHMs, format: pouchGBTM, offset: o.tm, size: 57, maxCount: 99, allowed: gen2Items(crystal, PocketTMHMs), hmMin: 243, hmMax: 249},
		{typ: PocketItems, format: pouchGBList, offset: o.item, size: 20, maxCount: 99, allowed: gen2Items(crystal, PocketItems)},
		{typ: PocketKeyItems, format: pouchGBKeys, offset: o.key, size: 26, maxCount: 99, allowed: gen2Items(crystal, PocketKeyItems)},
		{typ: PocketBalls, format: pouchGBList, offset: o.ball, size: 12, maxCount: 99, allowed: gen2Items(crystal, PocketBalls)},
		{typ: PocketPC, format: pouchGBList, offset: o.pc, size: 50, maxCount: 99, allowed: gen2Items(crystal, PocketPC)},
	}
}

func gen3Defs(kind gen3Kind, key uint32) []pouchDef {
	k := uint16(key)
	hm := func(d pouchDef) pouchDef { d.hmMin, d.hmMax = 339, 346; return d }
	switch kind {
	case kindRS:
		pc := concat(items3RSGeneral, items3RSKey, items3RSBerry, items3RSBalls, items3RSMachine)
		return []pouchDef{
			{typ: PocketItems, format: pouchGBA, offset: 0x0C8, size: 20, maxCount: 99, allowed: items3RSGeneral},
			{typ: PocketKeyItems, format: pouchGBA, offset: 0x118, size: 20, maxCount: 1, allowed: items3RSKey},
			{typ: PocketBalls, format: pouchGBA, offset: 0x168, size: 16, maxCount: 99, allowed: items3RSBalls},
			hm(pouchDef{typ: PocketTMHMs, format: pouchGBA, offset: 0x1A8, size: 64, maxCount: 99, allowed: items3RSMachine}),
			{typ: PocketBerries, format: pouchGBA, offset: 0x2A8, size: 46, maxCount: 999, allowed: items3RSBerry},
			{typ: PocketPC, format: pouchGBA, offset: 0x000, size: 50, maxCount: 999, allowed: pc},
		}
	case kindE:
		pc := concat(items3RSGeneral, items3RSBerry, items3RSBalls, items3RSMachine)
		return []pouchDef{
			{typ: PocketItems, format: pouchGBA, offset: 0x0C8, size: 30, maxCount: 99, allowed: items3RSGeneral, key: k},
			{typ: PocketKeyItems, format: pouchGBA, offset: 0x140, size: 30, maxCount: 1, allowed: items3EKey, key: k},
			{typ: PocketBalls, format: pouchGBA, offset: 0x1B8, size: 16, maxCount: 99, allowed: items3RSBalls, key: k},
			hm(pouchDef{typ: PocketTMHMs, format: pouchGBA, offset: 0x1F8, size: 64, maxCount: 99, allowed: items3RSMachine, key: k}),
			{typ: PocketBerries, format: pouchGBA, offset: 0x2F8, size: 46, maxCount: 999, allowed: items3RSBerry, key: k},
			{typ: PocketPC, format: pouchGBA, offset: 0x000, size: 50, maxCount: 999, allowed: pc},
		}
	}
	return []pouchDef{
		{typ: PocketItems, format: pouchGBA, offset: 0x078, size: 42, maxCount: 999, allowed: items3RSGeneral, key: k},
		{typ: PocketKeyItems, format: pouchGBA, offset: 0x120, size: 30, maxCount: 1, allowed: items3FRLGKey, key: k},
		{typ: PocketBalls, format: pouchGBA, offset: 0x198, size: 13, maxCount: 999, allowed: items3RSBalls, key: k},
		hm(pouchDef{typ: PocketTMHMs, format: pouchGBA, offset: 0x1CC, size: 58, maxCount: 999, allowed: items3RSMachine, key: k}),
		{typ: PocketBerries, format: pouchGBA, offset: 0x2B4, size: 43, maxCount: 999, allowed: items3RSBerry, key: k},
		{typ: PocketPC, format: pouchGBA, offset: 0x000, size: 30, maxCount: 999, allowed: items3RSGeneral},
	}
}

// HeldItemIDs returns the item ids a Pokémon may hold in a game.
func HeldItemIDs(g Game) []uint16 {
	switch g.Generation() {
	case 2:
		return concat(items2General, items2Balls, items2Machine[:50])
	case 3:
		return concat(items3RSGeneral, items3RSBalls, items3RSBerry, items3RSMachine[:50])
	}
	return nil
}
