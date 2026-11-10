// Package engine implements the pokegarden save-engine JSON protocol on top of
// the pokeedit library. It is a drop-in replacement for the former .NET
// (PKHeX.Core) bridge: one JSON request in, one JSON response out. No save
// path is ever read or written; save bytes travel base64-encoded.
package engine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	pe "github.com/8bit-necro/go-pokeedit"
)

// Request is a decoded protocol request.
type Request map[string]json.RawMessage

// Response is a protocol response (or {"error": "..."}).
type Response map[string]any

// Run decodes one JSON request, handles it and returns the JSON response. The
// boolean is false when the response is an error (the bridge exits with 1).
func Run(input []byte) ([]byte, bool) {
	var req Request
	var resp Response
	ok := true
	if err := json.Unmarshal(input, &req); err != nil {
		resp, ok = Response{"error": err.Error()}, false
	} else if r, err := Handle(req); err != nil {
		resp, ok = Response{"error": err.Error()}, false
	} else {
		resp = r
	}
	out, _ := json.Marshal(resp)
	return out, ok
}

// ---------------------------------------------------------------------------
// request helpers

func (r Request) has(key string) bool {
	v, ok := r[key]
	return ok && string(v) != "null"
}

func (r Request) text(key string) (string, error) {
	var s string
	if v, ok := r[key]; !ok || json.Unmarshal(v, &s) != nil {
		return "", fmt.Errorf("Missing %s", key)
	}
	return s, nil
}

func (r Request) number(key string) (int, error) {
	v, ok := r[key]
	if !ok {
		return 0, fmt.Errorf("The given key '%s' was not present in the dictionary.", key)
	}
	var n json.Number
	if err := json.Unmarshal(v, &n); err != nil {
		return 0, fmt.Errorf("The requested operation requires an element of type 'Number', but the target element has type 'String'.")
	}
	i, err := n.Int64()
	if err != nil {
		return 0, fmt.Errorf("Either the JsonElement name or value is not a valid number.")
	}
	return int(i), nil
}

func (r Request) boolean(key string) bool {
	var b bool
	if v, ok := r[key]; ok {
		_ = json.Unmarshal(v, &b)
	}
	return b
}

func (r Request) object(key string) (Request, error) {
	var o Request
	if v, ok := r[key]; !ok || json.Unmarshal(v, &o) != nil {
		return nil, fmt.Errorf("The given key '%s' was not present in the dictionary.", key)
	}
	return o, nil
}

func (r Request) bytes(key string) ([]byte, error) {
	s, err := r.text(key)
	if err != nil {
		return nil, err
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("The input is not a valid Base-64 string.")
	}
	return b, nil
}

// ---------------------------------------------------------------------------

// Handle dispatches a request.
func Handle(req Request) (Response, error) {
	action, err := req.text("action")
	if err != nil {
		return nil, err
	}
	switch action {
	case "training":
		return training(req)
	case "breed":
		return breed(req)
	case "profile":
		return profile(req)
	case "hatch":
		return hatch(req)
	case "entity":
		return entity(req)
	case "compatible":
		return compatible(req)
	case "fixture":
		return fixture(req)
	}
	sav, err := openSave(req)
	if err != nil {
		return nil, err
	}
	switch action {
	case "inspect":
		return Response{"summary": summary(sav)}, nil
	case "mutate":
		if err := mutate(sav, req); err != nil {
			return nil, err
		}
	case "deposit":
		return deposit(sav, req)
	case "withdraw":
		if err := withdraw(sav, req); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("Unknown engine action.")
	}
	return saveResponse(sav, nil)
}

func saveResponse(sav pe.Save, extra Response) (Response, error) {
	out, err := sav.Bytes()
	if err != nil {
		return nil, err
	}
	resp := Response{"save": base64.StdEncoding.EncodeToString(out)}
	for k, v := range extra {
		resp[k] = v
	}
	return resp, nil
}

func parseGame(s string) (pe.Game, error) { return pe.ParseGame(s) }

func openSave(req Request) (pe.Save, error) {
	raw, err := req.bytes("save")
	if err != nil {
		return nil, err
	}
	opts := pe.OpenOptions{}
	if req.has("game") {
		g, err := req.text("game")
		if err != nil {
			return nil, err
		}
		if opts.Game, err = parseGame(g); err != nil {
			return nil, err
		}
	}
	return pe.Open(raw, opts)
}

// ---------------------------------------------------------------------------
// Pokémon envelopes

func origin(payload Request) (pe.Game, error) {
	if payload.has("game") {
		s, err := payload.text("game")
		if err != nil {
			return "", err
		}
		return parseGame(s)
	}
	return pe.RedBlue, nil
}

func entityOf(payload Request) (*pe.Pokemon, error) {
	var pk *pe.Pokemon
	if payload.has("format") {
		f, _ := payload.text("format")
		if f != "pkhex-v1" {
			return nil, fmt.Errorf("Unsupported Pokémon envelope.")
		}
		raw, err := payload.bytes("data")
		if err != nil {
			return nil, err
		}
		if pk, err = pe.ParsePokemon(raw); err != nil {
			return nil, fmt.Errorf("Invalid Pokémon record.")
		}
		if _, err := origin(payload); err != nil {
			return nil, err
		}
	} else {
		core, _ := payload.text("core")
		ot, _ := payload.text("ot")
		nick, _ := payload.text("nickname")
		var err error
		if pk, err = pe.NewPokemonFromParts(core, ot, nick); err != nil {
			return nil, err
		}
	}
	if err := pe.ValidateEntity(pk); err != nil {
		return nil, err
	}
	return pk, nil
}

func payloadOf(pk *pe.Pokemon, game pe.Game) Response {
	pe.PrepareExport(pk)
	return Response{
		"format": "pkhex-v1",
		"data":   base64.StdEncoding.EncodeToString(pk.Bytes()),
		"game":   string(game),
	}
}

func details(pk *pe.Pokemon) Response {
	thresholds := make([]uint32, 100)
	for i := range thresholds {
		thresholds[i] = pe.ExpForLevel(i+1, pk.Growth())
	}
	games := []string{}
	for _, g := range pe.CompatibleGames(pk) {
		games = append(games, string(g))
	}
	return Response{
		"species": pk.Species(), "dex": pk.Species(), "nickname": pk.Nickname(),
		"species_name": pe.SpeciesName(pk.Species()), "generation": pk.Format(),
		"xp": pk.EXP(), "egg": pk.IsEgg(), "level": pk.Level(), "friendship": pk.Friendship(),
		"japanese": pk.Format() <= 2 && pk.Japanese(), "compatible_games": games,
		"growth": pe.GrowthName(pk.Growth()), "max_xp": pe.ExpForLevel(100, pk.Growth()),
		"thresholds": thresholds,
	}
}

// ---------------------------------------------------------------------------
// stateless actions

func entity(req Request) (Response, error) {
	pl, err := req.object("payload")
	if err != nil {
		return nil, err
	}
	pk, err := entityOf(pl)
	if err != nil {
		return nil, err
	}
	if req.has("xp") {
		n, _ := req.number("xp")
		pk.SetEXP(uint32(n))
	}
	if req.has("friendship") {
		n, err := req.number("friendship")
		if err != nil {
			return nil, err
		}
		if n < 0 || n > 255 {
			return nil, fmt.Errorf("Arithmetic operation resulted in an overflow.")
		}
		pk.SetFriendship(n)
	}
	if err := (func() error {
		if pk.EXP() > pe.ExpForLevel(100, pk.Growth()) {
			return fmt.Errorf("Experience exceeds level 100.")
		}
		return nil
	})(); err != nil {
		return nil, err
	}
	g, err := origin(pl)
	if err != nil {
		return nil, err
	}
	return Response{"payload": payloadOf(pk, g), "details": details(pk)}, nil
}

func compatible(req Request) (Response, error) {
	gs, err := req.text("game")
	if err != nil {
		return nil, err
	}
	target, err := parseGame(gs)
	if err != nil {
		return nil, err
	}
	var payloads []Request
	if v, ok := req["payloads"]; ok {
		if err := json.Unmarshal(v, &payloads); err != nil {
			return nil, fmt.Errorf("Invalid payloads.")
		}
	}
	jp := req.boolean("japanese")
	matches := make([]bool, 0, len(payloads))
	for _, pl := range payloads {
		pk, err := entityOf(pl)
		if err != nil {
			matches = append(matches, false)
			continue
		}
		_, err = pk.ConvertFor(target.Generation(), jp)
		matches = append(matches, err == nil)
	}
	return Response{"compatible": matches}, nil
}

func hatch(req Request) (Response, error) {
	pl, err := req.object("payload")
	if err != nil {
		return nil, err
	}
	pk, err := entityOf(pl)
	if err != nil {
		return nil, err
	}
	g, err := origin(pl)
	if err != nil {
		return nil, err
	}
	if err := pe.Hatch(pk, g, pe.LanguageNone); err != nil {
		return nil, err
	}
	return Response{"payload": payloadOf(pk, g), "details": details(pk)}, nil
}

func profile(req Request) (Response, error) {
	pl, err := req.object("payload")
	if err != nil {
		return nil, err
	}
	pk, err := entityOf(pl)
	if err != nil {
		return nil, err
	}
	g, err := origin(pl)
	if err != nil {
		return nil, err
	}
	p := pe.ProfileOf(pk, g)
	labels := []string{"hp", "attack", "defense", "speed", "sp_atk", "sp_def"}
	pack := func(v [6]int) map[string]int {
		m := map[string]int{}
		for i, l := range labels {
			m[l] = v[i]
		}
		return m
	}
	moves := []Response{}
	for _, m := range p.Moves {
		moves = append(moves, Response{"slot": m.Slot, "move": m.Move, "name": m.Name, "pp": m.PP})
	}
	var nature, held any
	if p.Nature != "" {
		nature = p.Nature
	}
	if p.HeldItem != "" {
		held = p.HeldItem
	}
	return Response{
		"species_name": p.SpeciesName, "egg": p.Egg, "level": p.Level, "gender": p.Gender,
		"nature": nature, "shiny": p.Shiny, "held_item": held, "trainer": p.Trainer,
		"trainer_id": p.TrainerID, "stats": pack(p.Stats), "ivs": pack(p.IVs), "evs": pack(p.EVs),
		"moves": moves,
	}, nil
}

func breed(req Request) (Response, error) {
	fp, err := req.object("first")
	if err != nil {
		return nil, err
	}
	sp, err := req.object("second")
	if err != nil {
		return nil, err
	}
	a, err := entityOf(fp)
	if err != nil {
		return nil, err
	}
	b, err := entityOf(sp)
	if err != nil {
		return nil, err
	}
	var gameErr error
	var motherGame pe.Game
	res, err := pe.Breed(a, b, func(mother *pe.Pokemon) pe.Game {
		pl := fp
		if mother == b {
			pl = sp
		}
		motherGame, gameErr = origin(pl)
		return motherGame
	}, nil)
	if err != nil {
		return nil, err
	}
	if gameErr != nil {
		return nil, gameErr
	}
	inherited := res.Inherited
	if inherited == nil {
		inherited = []string{}
	}
	return Response{
		"payload": payloadOf(res.Egg, motherGame), "details": details(res.Egg),
		"mother": res.Mother, "inherited": inherited,
	}, nil
}

func training(req Request) (Response, error) {
	pl, err := req.object("payload")
	if err != nil {
		return nil, err
	}
	pk, err := entityOf(pl)
	if err != nil {
		return nil, err
	}
	if err := (func() error {
		if pk.IsEgg() {
			return fmt.Errorf("Let this egg hatch before training.")
		}
		return nil
	})(); err != nil {
		return nil, err
	}
	game, err := origin(pl)
	if err != nil {
		return nil, err
	}
	if req.has("xp") {
		n, _ := req.number("xp")
		pk.SetEXP(uint32(n))
	}
	if pk.EXP() > pe.ExpForLevel(100, pk.Growth()) {
		return nil, fmt.Errorf("Experience exceeds level 100.")
	}
	mode, err := req.text("mode")
	if err != nil {
		return nil, err
	}
	nowSec, err := req.number("now")
	if err != nil {
		return nil, err
	}
	now := time.Unix(int64(nowSec), 0)
	var remembered []int
	if v, ok := req["remembered"]; ok && string(v) != "null" {
		var raw []int
		if json.Unmarshal(v, &raw) == nil {
			remembered = raw
		}
	}
	pool := pe.MovePool(pk, game, remembered)
	switch mode {
	case "options":
		evos := []Response{}
		for _, c := range pe.EvolutionChoices(pk, game, now) {
			evos = append(evos, Response{"target": c.Target, "name": c.Name, "ready": c.Ready,
				"requirement": c.Requirement, "consumes_item": c.ConsumesItem})
		}
		moves := pk.Moves()
		cur := []Response{}
		for slot, m := range moves {
			cur = append(cur, Response{"slot": slot, "move": m, "name": pe.MoveName(m)})
		}
		relearn := []Response{}
		for _, m := range pool {
			known := false
			for _, c := range moves {
				if c == m {
					known = true
				}
			}
			if !known {
				relearn = append(relearn, Response{"move": m, "name": pe.MoveName(m)})
			}
		}
		return Response{
			"game": string(game), "level": pk.Level(), "friendship": pk.Friendship(),
			"evolutions": evos, "moves": cur, "relearn": relearn, "remembered": pool,
		}, nil
	case "evolve":
		target, err := req.number("target")
		if err != nil {
			return nil, err
		}
		if err := pe.Evolve(pk, game, target, now); err != nil {
			return nil, err
		}
	case "relearn":
		move, err := req.number("move")
		if err != nil {
			return nil, err
		}
		slot, err := req.number("slot")
		if err != nil {
			return nil, err
		}
		if err := pe.Relearn(pk, game, slot, move, remembered); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("Unknown training action.")
	}
	var out any
	if !pl.has("format") && pk.Format() == 1 {
		core, ot, nick := pk.LegacyParts()
		out = Response{"core": hexLower(core), "ot": hexLower(ot), "nickname": hexLower(nick)}
	} else {
		out = payloadOf(pk, game)
	}
	return Response{"payload": out, "details": details(pk), "remembered": pool}, nil
}

func hexLower(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2], out[i*2+1] = digits[v>>4], digits[v&15]
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// fixtures

func fixture(req Request) (Response, error) {
	gs, err := req.text("game")
	if err != nil {
		return nil, err
	}
	game, err := parseGame(gs)
	if err != nil {
		return nil, err
	}
	sav, err := pe.NewSave(game, pe.NewSaveOptions{OT: "GARDEN", TID: 1234})
	if err != nil {
		return nil, err
	}
	species := []int{1, 25}
	if req.has("species") {
		species = nil
		_ = json.Unmarshal(req["species"], &species)
	}
	var genders []int
	if req.has("genders") {
		_ = json.Unmarshal(req["genders"], &genders)
	}
	for index, sp := range species {
		pk := sav.BlankPokemon()
		pk.SetSpecies(sp)
		pk.SetLevel(5)
		pk.SetLanguage(pe.LanguageEnglish)
		if index < len(genders) && pk.Format() >= 2 {
			for trial := 0; trial < 256 && pk.Gender() != genders[index]; trial++ {
				if pk.Format() == 3 {
					pk.SetPID(uint32(trial))
				} else {
					pk.SetIV(pe.StatAttack, trial&15)
				}
			}
		}
		pk.SetNickname(pe.DefaultNickname(sp, pe.LanguageEnglish, sav.Generation()))
		pk.SetOTName(sav.OT())
		pk.SetTID(sav.TID())
		pk.SetMove(0, 33)
		pk.SetMovePP(0, 35)
		pk.ResetPartyStats()
		pk.RefreshChecksum()
		if err := pe.PlaceNew(sav, pk, -1); err != nil {
			return nil, err
		}
	}
	visitor, err := sav.Party(0)
	if err != nil {
		return nil, err
	}
	if req.boolean("egg") {
		if sav.Generation() < 2 {
			return nil, fmt.Errorf("Generation I has no eggs.")
		}
		visitor.SetEgg(true)
		visitor.SetFriendship(2)
		visitor.RefreshChecksum()
	}
	if err := pe.PlaceNew(sav, visitor, 0); err != nil {
		return nil, err
	}
	if s1, ok := sav.(*pe.Save1); ok && game == pe.Yellow {
		s1.SetStarter(0x54)
	}
	return saveResponse(sav, nil)
}

// ---------------------------------------------------------------------------
// deposit / withdraw

func slotOf(req Request) (pe.Slot, error) {
	at := pe.PartySlot(0)
	if req.has("box") {
		b, err := req.number("box")
		if err != nil {
			return at, err
		}
		at.Box = b
	}
	s, err := req.number("slot")
	if err != nil {
		return at, err
	}
	at.Slot = s
	return at, nil
}

func deposit(sav pe.Save, req Request) (Response, error) {
	at, err := slotOf(req)
	if err != nil {
		return nil, err
	}
	if at.Slot < 0 || (at.Box < 0 && at.Slot >= sav.PartyCount()) || (at.Box >= 0 && at.Slot >= sav.BoxSlotCount()) {
		return nil, fmt.Errorf("Select an occupied Pokémon slot.")
	}
	pk, err := pe.Deposit(sav, at)
	if err != nil {
		return nil, err
	}
	return saveResponse(sav, Response{"payload": payloadOf(pk, sav.Game())})
}

func withdraw(sav pe.Save, req Request) error {
	pl, err := req.object("payload")
	if err != nil {
		return err
	}
	pk, err := entityOf(pl)
	if err != nil {
		return err
	}
	box := -1
	if req.has("box") {
		if box, err = req.number("box"); err != nil {
			return err
		}
	}
	return pe.Withdraw(sav, pk, box)
}

func jsonUnmarshal(b json.RawMessage, v any) error { return json.Unmarshal(b, v) }
