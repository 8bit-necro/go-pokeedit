package engine

import (
	"fmt"
	"strconv"

	pe "github.com/8bit-necro/go-pokeedit"
)

var monFieldOrder = []string{
	"Nickname", "Species", "CurrentLevel", "Stat_HPCurrent", "Status_Condition", "OriginalTrainerName",
	"TID16", "SID16", "HeldItem", "CurrentFriendship", "Nature", "Gender",
	"Move1", "Move2", "Move3", "Move4",
	"IV_HP", "IV_ATK", "IV_DEF", "IV_SPE", "IV_SPA", "IV_SPD",
	"EV_HP", "EV_ATK", "EV_DEF", "EV_SPE", "EV_SPA", "EV_SPD",
}

var statIndex = map[string]int{"HP": pe.StatHP, "ATK": pe.StatAttack, "DEF": pe.StatDefense,
	"SPE": pe.StatSpeed, "SPA": pe.StatSpecialAttack, "SPD": pe.StatSpecialDefense}

var trainerFields = map[string]bool{"OT": true, "Money": true, "TID16": true, "SID16": true,
	"Gender": true, "Badges": true, "Coin": true, "Coins": true, "RivalName": true}

func monField(pk *pe.Pokemon, name string) any {
	switch name {
	case "Nickname":
		return pk.Nickname()
	case "Species":
		return pk.Species()
	case "CurrentLevel":
		return pk.Level()
	case "Stat_HPCurrent":
		return pk.HP()
	case "Status_Condition":
		return pk.Status()
	case "OriginalTrainerName":
		return pk.OTName()
	case "TID16":
		return pk.TID()
	case "SID16":
		return pk.SID()
	case "HeldItem":
		return pk.HeldItem()
	case "CurrentFriendship":
		return pk.Friendship()
	case "Nature":
		if n := pk.Nature(); n >= 0 {
			return n
		}
		return 0
	case "Gender":
		return pk.Gender()
	}
	if len(name) == 5 && name[:4] == "Move" {
		return pk.Move(int(name[4] - '1'))
	}
	if len(name) > 3 && name[:3] == "IV_" {
		return pk.IV(statIndex[name[3:]])
	}
	if len(name) > 3 && name[:3] == "EV_" {
		return pk.EV(statIndex[name[3:]])
	}
	return nil
}

func mon(sav pe.Save, pk *pe.Pokemon, slot int) Response {
	values := map[string]any{}
	for _, f := range monFieldOrder {
		values[f] = monField(pk, f)
	}
	return Response{
		"slot": slot, "species": pk.Species(), "nickname": pk.Nickname(), "level": pk.Level(),
		"xp": pk.EXP(), "egg": pk.IsEgg(), "values": values,
		"stats": pe.CalcStatsFor(pk, sav.Game()),
	}
}

func summary(sav pe.Save) Response {
	gen := sav.Generation()
	trainer := map[string]any{
		"OT": sav.OT(), "Money": sav.Money(), "TID16": sav.TID(), "SID16": sav.SID(),
		"Gender": sav.Gender(), "Badges": sav.Badges(), "Coin": sav.Coins(),
	}
	if name, ok := sav.RivalName(); ok {
		trainer["RivalName"] = name
	}
	party := []Response{}
	for i := 0; i < sav.PartyCount(); i++ {
		if pk, err := sav.Party(i); err == nil {
			party = append(party, mon(sav, pk, i))
		}
	}
	boxes := make([][]Response, sav.BoxCount())
	for b := range boxes {
		boxes[b] = []Response{}
		for s := 0; s < sav.BoxSlotCount(); s++ {
			if pk, err := sav.Box(b, s); err == nil && pk != nil && pk.Species() != 0 {
				boxes[b] = append(boxes[b], mon(sav, pk, s))
			}
		}
	}
	species := map[string]string{}
	for i := 1; i <= sav.MaxSpecies(); i++ {
		if sav.Personal().InGame(i) {
			species[strconv.Itoa(i)] = pe.SpeciesName(i)
		}
	}
	moves := map[string]string{}
	for i := 0; i <= sav.MaxMove(); i++ {
		moves[strconv.Itoa(i)] = pe.MoveName(i)
	}
	names := pe.ItemNames(gen)
	held := map[string]string{}
	for _, id := range sav.HeldItems() {
		if int(id) < len(names) {
			held[strconv.Itoa(int(id))] = names[id]
		}
	}
	pockets := []Response{}
	for _, p := range sav.Pockets() {
		allowed := map[string]string{}
		for _, id := range p.Allowed {
			if int(id) < len(names) {
				allowed[strconv.Itoa(int(id))] = names[id]
			}
		}
		entries := []Response{}
		for _, e := range p.Entries {
			name := "Unknown"
			if e.Item < len(names) {
				name = names[e.Item]
			}
			entries = append(entries, Response{"slot": e.Slot, "item": e.Item, "name": name, "quantity": e.Count})
		}
		pockets = append(pockets, Response{"index": p.Index, "name": p.Type, "capacity": p.Capacity,
			"max_count": p.MaxCount, "allowed": allowed, "entries": entries})
	}
	dex := make([]Response, 0, sav.MaxSpecies())
	for i := 1; i <= sav.MaxSpecies(); i++ {
		dex = append(dex, Response{"species": i, "seen": sav.Seen(i), "owned": sav.Caught(i)})
	}
	return Response{
		"game": string(sav.Game()), "generation": gen, "japanese": gen <= 2 && sav.Japanese(),
		"current_box": sav.CurrentBox(), "box_count": sav.BoxCount(), "box_capacity": sav.BoxSlotCount(),
		"max_species": sav.MaxSpecies(), "max_move": sav.MaxMove(), "max_iv": sav.MaxIV(), "max_ev": sav.MaxEV(),
		"max_money": sav.MaxMoney(), "nickname_length": sav.MaxNicknameLength(), "trainer_length": sav.MaxTrainerLength(),
		"trainer": trainer, "checksum_ok": sav.ChecksumsValid(), "party": party, "boxes": boxes,
		"species": species, "moves": moves, "held_items": held, "pockets": pockets, "dex": dex,
	}
}

// ---------------------------------------------------------------------------

func mutate(sav pe.Save, r Request) error {
	scope, err := r.text("scope")
	if err != nil {
		return err
	}
	switch scope {
	case "trainer":
		return mutateTrainer(sav, r)
	case "dex":
		sp, err := r.number("species")
		if err != nil {
			return err
		}
		if sp <= 0 || sp > sav.MaxSpecies() {
			return fmt.Errorf("Species is not supported by this game.")
		}
		seen, owned := r.boolean("seen"), r.boolean("owned")
		sav.SetSeen(sp, seen || owned)
		sav.SetCaught(sp, owned)
		return nil
	case "inventory":
		return mutateInventory(sav, r)
	}
	box := -1
	if r.has("box") {
		if box, err = r.number("box"); err != nil {
			return err
		}
	}
	if scope == "add" {
		species, err := r.number("species")
		if err != nil {
			return err
		}
		level, err := r.number("level")
		if err != nil {
			return err
		}
		_, err = pe.AddPokemon(sav, species, level, box)
		return err
	}
	slot, err := r.number("slot")
	if err != nil {
		return err
	}
	if slot < 0 || (box < 0 && slot >= sav.PartyCount()) || (box >= 0 && slot >= sav.BoxSlotCount()) {
		return fmt.Errorf("Select an occupied Pokémon slot.")
	}
	if box >= sav.BoxCount() {
		return fmt.Errorf("Invalid box.")
	}
	at := pe.Slot{Box: box, Slot: slot}
	pk, err := pe.GetPokemon(sav, at)
	if err != nil {
		return err
	}
	if scope == "heal" {
		return pe.HealPokemon(sav, at)
	}
	field, err := r.text("field")
	if err != nil {
		return err
	}
	if err := setMonField(sav, pk, field, r); err != nil {
		return err
	}
	return pe.SetPokemon(sav, at, pk)
}

func isMonField(name string) bool {
	for _, f := range monFieldOrder {
		if f == name {
			return true
		}
	}
	return false
}

func setMonField(sav pe.Save, pk *pe.Pokemon, field string, r Request) error {
	if !isMonField(field) {
		return fmt.Errorf("Unsupported Pokémon field.")
	}
	isString := field == "Nickname" || field == "OriginalTrainerName"
	var text string
	var value int
	if isString {
		var err error
		if text, err = r.text("value"); err != nil {
			return err
		}
		n := len([]rune(text))
		if field == "Nickname" && !(n > 0 && n <= sav.MaxNicknameLength()) {
			return fmt.Errorf("Nickname is too long for this game.")
		}
		if field == "OriginalTrainerName" && !(n > 0 && n <= sav.MaxTrainerLength()) {
			return fmt.Errorf("OT name is too long for this game.")
		}
	} else {
		var err error
		if value, err = r.number("value"); err != nil {
			return err
		}
	}
	switch {
	case len(field) > 3 && field[:3] == "IV_":
		if value < 0 || value > sav.MaxIV() {
			return fmt.Errorf("DV/IV is out of range for this game.")
		}
	case len(field) > 3 && field[:3] == "EV_":
		if value < 0 || value > sav.MaxEV() {
			return fmt.Errorf("Stat experience/EV is out of range for this game.")
		}
	case field == "HeldItem":
		ok := value == 0
		for _, id := range sav.HeldItems() {
			if int(id) == value {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("Held item is unsupported by this game.")
		}
	case field == "Species":
		if !(value > 0 && sav.Personal().InGame(value)) {
			return fmt.Errorf("Species is not supported by this game.")
		}
	case field == "CurrentLevel":
		if value < 2 || value > 100 {
			return fmt.Errorf("Level must be 2–100.")
		}
	case len(field) == 5 && field[:4] == "Move":
		if value < 0 || value > sav.MaxMove() {
			return fmt.Errorf("Move is not supported by this game.")
		}
	}
	level := pk.Level()
	switch field {
	case "Nickname":
		pk.SetNickname(text)
	case "OriginalTrainerName":
		pk.SetOTName(text)
	case "Species":
		pk.SetSpecies(value)
	case "CurrentLevel":
		pk.SetLevel(value)
	case "Stat_HPCurrent":
		pk.SetHP(value)
	case "Status_Condition":
		pk.SetStatus(value)
	case "TID16":
		pk.SetTID(value)
	case "SID16":
		pk.SetSID(value)
	case "HeldItem":
		pk.SetHeldItem(value)
	case "CurrentFriendship":
		pk.SetFriendship(value)
	case "Nature", "Gender": // derived values; writes are ignored like PKHeX
	default:
		switch {
		case len(field) == 5 && field[:4] == "Move":
			pk.SetMove(int(field[4]-'1'), value)
		case field[:3] == "IV_":
			pk.SetIV(statIndex[field[3:]], value)
		case field[:3] == "EV_":
			pk.SetEV(statIndex[field[3:]], value)
		}
	}
	if sav.Generation() == 3 && len(field) > 3 && field[:3] == "EV_" && pk.EVTotal() > 510 {
		return fmt.Errorf("Total EVs cannot exceed 510.")
	}
	if field == "Species" {
		pk.SetLevel(level)
	}
	if field == "Species" || field == "CurrentLevel" || (len(field) > 3 && (field[:3] == "IV_" || field[:3] == "EV_")) {
		pe.Recalculate(sav, pk)
	}
	if len(field) == 5 && field[:4] == "Move" {
		pk.HealPP()
	}
	return nil
}

func mutateTrainer(sav pe.Save, r Request) error {
	field, err := r.text("field")
	if err != nil {
		return err
	}
	if !trainerFields[field] {
		return fmt.Errorf("Unsupported trainer field.")
	}
	if field == "OT" || field == "RivalName" {
		v, err := r.text("value")
		if err != nil {
			return err
		}
		if n := len([]rune(v)); n == 0 || n > sav.MaxTrainerLength() {
			return fmt.Errorf("Trainer/rival name is too long for this game.")
		}
		if field == "OT" {
			return sav.SetOT(v)
		}
		if _, ok := sav.RivalName(); !ok {
			return fmt.Errorf("Unsupported field.")
		}
		return sav.SetRivalName(v)
	}
	v, err := r.number("value")
	if err != nil {
		return err
	}
	switch field {
	case "Money":
		if v < 0 || v > sav.MaxMoney() {
			return fmt.Errorf("Money is out of range.")
		}
		return sav.SetMoney(uint32(v))
	case "TID16":
		sav.SetTID(v & 0xFFFF)
	case "SID16":
		sav.SetSID(v & 0xFFFF)
	case "Gender":
		sav.SetGender(v)
	case "Badges":
		sav.SetBadges(v)
	case "Coin":
		if v < 0 {
			return fmt.Errorf("Value was either too large or too small for a UInt32.")
		}
		if v > 9999 {
			v = 9999
		}
		return sav.SetCoins(uint32(v))
	default:
		return fmt.Errorf("Unsupported field.")
	}
	return nil
}

func mutateInventory(sav pe.Save, r Request) error {
	pocket, err := r.number("pocket")
	if err != nil {
		return err
	}
	if pocket < 0 || pocket >= len(sav.Pockets()) {
		return fmt.Errorf("Invalid inventory pocket.")
	}
	var raw []Request
	if v, ok := r["entries"]; ok {
		if err := jsonUnmarshal(v, &raw); err != nil {
			return err
		}
	}
	entries := make([]pe.ItemStack, 0, len(raw))
	for _, e := range raw {
		item, err := e.number("item")
		if err != nil {
			return err
		}
		qty, err := e.number("quantity")
		if err != nil {
			return err
		}
		entries = append(entries, pe.ItemStack{Item: item, Count: qty})
	}
	return sav.SetPocket(pocket, entries)
}
