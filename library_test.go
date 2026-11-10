package pokeedit

import (
	"bytes"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

func TestGen3EncryptionRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 500; i++ {
		raw := make([]byte, size3Stored)
		for j := range raw {
			raw[j] = byte(rng.UintN(256))
		}
		orig := append([]byte(nil), raw...)
		encrypt3(raw)
		if i > 0 && bytes.Equal(raw[32:], orig[32:]) && le32(orig) != 0 && le32(orig)^le32(orig[4:]) != 0 {
			t.Fatalf("encryption did nothing for %x", orig[:8])
		}
		decrypt3(raw)
		if !bytes.Equal(raw, orig) {
			t.Fatalf("round trip failed for PID %08x", le32(orig))
		}
	}
	for sv := byte(0); sv < 24; sv++ {
		if blockPosition[blockPositionInvert[sv]] == blockPosition[sv] && sv != 0 && blockPositionInvert[sv] == sv {
			continue // involutions are fine
		}
	}
}

func TestSpeciesConversionIsBijective(t *testing.T) {
	for s := 1; s <= 386; s++ {
		if got := nationalSpecies3(internalSpecies3(s)); got != s {
			t.Fatalf("gen3 species %d -> %d -> %d", s, internalSpecies3(s), got)
		}
	}
	for s := 1; s <= 151; s++ {
		if got := nationalSpecies1(internalSpecies1(s)); got != s {
			t.Fatalf("gen1 species %d -> %d", s, got)
		}
	}
	if internalSpecies3(277) != 283 && nationalSpecies3(277) == 0 {
		t.Fatal("hoenn remap missing")
	}
}

func TestStringCodecs(t *testing.T) {
	buf := make([]byte, 11)
	for _, tc := range []struct {
		gen  int
		lang Language
		text string
	}{
		{1, LanguageEnglish, "PIKACHU"}, {1, LanguageEnglish, "RED ♂"}, {2, LanguageEnglish, "CRYSTAL"},
		{2, LanguageFrench, "Héros"}, {2, LanguageItalian, "GIOVANNI"}, {2, LanguageJapanese, "ピカチュウ"},
	} {
		encodeGB(buf, tc.text, 10, tc.gen, tc.lang, clear50)
		if got := decodeGB(buf, tc.gen, tc.lang); got != tc.text {
			t.Errorf("gen%d %v: %q -> %q", tc.gen, tc.lang, tc.text, got)
		}
	}
	for _, text := range []string{"Treecko", "ANDY", "Mr. Mime", "Nidoran♀", "O'Neil", "Zoë"} {
		encodeG3(buf, text, 10, LanguageEnglish, clearFF)
		if got := decodeG3(buf, LanguageEnglish); got != strings.ReplaceAll(text, "ë", "ë") {
			t.Errorf("gen3 %q -> %q", text, got)
		}
	}
	// terminator handling
	encodeGB(buf, "ASH", 7, 1, LanguageEnglish, clearZero)
	if buf[3] != gbTerminator || buf[4] != 0 {
		t.Errorf("unexpected tail %x", buf[:6])
	}
	if got := defaultSpeciesNickname(122, LanguageEnglish, 1); got != "MR․MIME" {
		t.Errorf("Mr. Mime = %q", got)
	}
	if got := defaultSpeciesNickname(83, LanguageEnglish, 3); got != "FARFETCH'D" {
		t.Errorf("Farfetch'd gen3 = %q", got)
	}
}

func TestExperienceTables(t *testing.T) {
	want := map[int]uint32{GrowthMediumFast: 1000000, GrowthErratic: 600000, GrowthFluctuating: 1640000,
		GrowthMediumSlow: 1059860, GrowthFast: 800000, GrowthSlow: 1250000}
	for g, exp := range want {
		if got := ExpForLevel(100, g); got != exp {
			t.Errorf("growth %d level 100 = %d, want %d", g, got, exp)
		}
		for l := 1; l <= 100; l++ {
			if LevelForExp(ExpForLevel(l, g), g) != l {
				t.Fatalf("growth %d level %d does not round-trip", g, l)
			}
		}
	}
}

func TestStatCalculation(t *testing.T) {
	// Generation 1: Pikachu L50 with perfect DVs and no stat experience.
	pk := NewPokemon(1, false)
	pk.SetSpecies(25)
	pk.SetLevel(50)
	pk.SetDV16(0xFFFF)
	pk.ResetPartyStats()
	if got, want := pk.Stats(), [6]int{110, 75, 50, 110, 70, 70}; got != want {
		t.Errorf("gen1 pikachu stats = %v, want %v", got, want)
	}
	if pk.HP() != pk.MaxHP() {
		t.Error("ResetPartyStats should fill HP")
	}
	// Generation 3: Mudkip L50, neutral nature, 31 IVs, no EVs.
	m := NewPokemon(3, false)
	m.SetSpecies(258)
	m.SetLevel(50)
	m.SetPID(0) // Hardy
	for i := 0; i < 6; i++ {
		m.SetIV(i, 31)
	}
	m.ResetPartyStats()
	if got, want := m.Stats(), [6]int{125, 90, 70, 60, 70, 70}; got != want {
		t.Errorf("gen3 mudkip stats = %v, want %v", got, want)
	}
	// Adamant (+Atk −SpA) is nature index 3.
	m.SetPID(3)
	m.ResetPartyStats()
	if got := m.Stats(); got[1] != 90*11/10 || got[4] != 70*9/10 {
		t.Errorf("adamant stats = %v", got)
	}
}

func TestEvolutionAndLearnsetData(t *testing.T) {
	evos := EvolutionsOf(1, 4) // Charmander
	if len(evos) != 1 || evos[0].Species != 5 || evos[0].Method != EvoLevelUp || evos[0].Level != 16 {
		t.Errorf("charmander gen1 evolutions = %+v", evos)
	}
	if got := EvolutionsOf(3, 265); len(got) != 2 { // Wurmple branches
		t.Errorf("wurmple = %+v", got)
	}
	if base := baseSpecies(3, 7); base != 7 {
		t.Errorf("squirtle base = %d", base)
	}
	if base := baseSpecies(3, 9); base != 7 {
		t.Errorf("blastoise base = %d", base)
	}
	ls := LearnsetOf(RedBlue, 1)
	found := false
	for i, m := range ls.Moves {
		if m == 73 && ls.Levels[i] == 7 { // Leech Seed at 7
			found = true
		}
	}
	if !found {
		t.Errorf("bulbasaur learnset = %v / %v", ls.Moves, ls.Levels)
	}
	if bp := BasePP(3, 33); bp != 35 {
		t.Errorf("tackle pp = %d", bp)
	}
}

func TestCrossGenerationConversion(t *testing.T) {
	red := mustSave(t, RedBlue)
	pk, err := AddPokemon(red, 25, 30, -1)
	if err != nil {
		t.Fatal(err)
	}
	pk.SetHP(7)
	conv, err := pk.ConvertFor(2, false)
	if err != nil {
		t.Fatal(err)
	}
	if conv.Format() != 2 || conv.Species() != 25 || conv.Level() != 30 || conv.Nickname() != "PIKACHU" {
		t.Fatalf("converted = fmt %d #%d L%d %q", conv.Format(), conv.Species(), conv.Level(), conv.Nickname())
	}
	if conv.HP() != 7 || conv.Friendship() != 70 {
		t.Errorf("converted hp=%d friendship=%d", conv.HP(), conv.Friendship())
	}
	back, err := conv.ConvertFor(1, false)
	if err != nil || back.Species() != 25 || back.Level() != 30 {
		t.Fatalf("round trip: %v %v", back, err)
	}
	conv.SetHeldItem(3)
	if _, err := conv.ConvertFor(1, false); err == nil {
		t.Error("held item should block return to generation 1")
	}
	gsc := mustSave(t, GoldSilver)
	if _, err := AddPokemon(gsc, 152, 5, -1); err != nil {
		t.Fatal(err)
	}
	johto, _ := gsc.Party(0)
	if _, err := johto.ConvertFor(1, false); err == nil {
		t.Error("Chikorita does not exist in generation 1")
	}
	if _, err := pk.ConvertFor(3, false); err == nil {
		t.Error("GB -> GBA must be refused")
	}
	if _, err := pk.ConvertFor(1, true); err == nil {
		t.Error("international -> Japanese must be refused")
	}
}

func TestDepositWithdrawFlow(t *testing.T) {
	for _, g := range []Game{RedBlue, Crystal, Emerald} {
		t.Run(string(g), func(t *testing.T) {
			sav := mustSave(t, g)
			sp := 4
			if g == Emerald {
				sp = 255
			}
			if _, err := AddPokemon(sav, sp, 12, -1); err != nil {
				t.Fatal(err)
			}
			if _, err := Deposit(sav, PartySlot(0)); err == nil {
				t.Fatal("the last party Pokémon must stay")
			}
			if _, err := AddPokemon(sav, sp+1, 13, -1); err != nil {
				t.Fatal(err)
			}
			pk, err := Deposit(sav, PartySlot(0))
			if err != nil {
				t.Fatal(err)
			}
			if sav.PartyCount() != 1 || pk.Species() != sp {
				t.Fatalf("deposit left party=%d, took #%d", sav.PartyCount(), pk.Species())
			}
			// it survives the envelope
			again, err := ParsePokemon(pk.Bytes())
			if err != nil || !bytes.Equal(again.Bytes(), pk.Bytes()) {
				t.Fatalf("envelope: %v", err)
			}
			if err := Withdraw(sav, again, 3); err != nil {
				t.Fatal(err)
			}
			got, _ := sav.Box(3, 0)
			if got == nil || got.Species() != sp || got.Nickname() != pk.Nickname() {
				t.Fatalf("box mon = %v", got)
			}
			if err := Withdraw(sav, again, -1); err != nil {
				t.Fatal(err)
			}
			for sav.PartyCount() < 6 {
				if err := Withdraw(sav, again, -1); err != nil {
					t.Fatal(err)
				}
			}
			if err := Withdraw(sav, again, -1); err == nil || err.Error() != "The party is full." {
				t.Fatalf("full party error = %v", err)
			}
		})
	}
}

func TestEvolutionRules(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	sav := mustSave(t, RedBlue)
	if _, err := AddPokemon(sav, 4, 15, -1); err != nil {
		t.Fatal(err)
	}
	pk, _ := sav.Party(0)
	if c := EvolutionChoices(pk, RedBlue, now); len(c) != 1 || c[0].Ready {
		t.Fatalf("level 15 charmander choices = %+v", c)
	}
	pk.SetLevel(16)
	if err := Evolve(pk, RedBlue, 5, now); err != nil {
		t.Fatal(err)
	}
	if pk.Species() != 5 || pk.Nickname() != "CHARMELEON" {
		t.Fatalf("evolved into #%d %q", pk.Species(), pk.Nickname())
	}
	if err := Evolve(pk, RedBlue, 6, now); err == nil {
		t.Fatal("level 16 charmeleon must not become charizard")
	}
	// Custom nicknames survive evolution.
	pk2, _ := sav.Party(0)
	pk2.SetLevel(20)
	pk2.SetNickname("BLAZE")
	if err := Evolve(pk2, RedBlue, 5, now); err != nil || pk2.Nickname() != "BLAZE" {
		t.Fatalf("custom nickname: %v %q", err, pk2.Nickname())
	}
	// Yellow's starter Pikachu cannot evolve.
	yellow := mustSave(t, Yellow)
	if _, err := AddPokemon(yellow, 25, 30, -1); err != nil {
		t.Fatal(err)
	}
	pika, _ := yellow.Party(0)
	if c := EvolutionChoices(pika, Yellow, now); len(c) != 1 || c[0].Ready {
		t.Fatalf("yellow pikachu = %+v", c)
	}
	// Friendship + time of day (Gold/Silver clock rules).
	gold := mustSave(t, Gold)
	if _, err := AddPokemon(gold, 133, 20, -1); err != nil { // Eevee
		t.Fatal(err)
	}
	eevee, _ := gold.Party(0)
	var espeon, umbreon EvolutionChoice
	for _, c := range EvolutionChoices(eevee, Gold, now) {
		switch c.Target {
		case 196:
			espeon = c
		case 197:
			umbreon = c
		}
	}
	if espeon.Ready || umbreon.Ready {
		t.Fatal("low friendship must block Espeon/Umbreon")
	}
	eevee.SetFriendship(255)
	for _, c := range EvolutionChoices(eevee, Gold, now) { // noon
		switch c.Target {
		case 196:
			espeon = c
		case 197:
			umbreon = c
		}
	}
	if !espeon.Ready || umbreon.Ready {
		t.Fatalf("at noon only Espeon should be ready: %+v %+v", espeon, umbreon)
	}
}

func TestRelearn(t *testing.T) {
	sav := mustSave(t, Crystal)
	if _, err := AddPokemon(sav, 152, 30, -1); err != nil {
		t.Fatal(err)
	}
	pk, _ := sav.Party(0)
	pool := MovePool(pk, Crystal, nil)
	if len(pool) < 5 {
		t.Fatalf("pool = %v", pool)
	}
	var fresh int
	known := pk.Moves()
	for _, m := range pool {
		if !containsInt(known[:], m) {
			fresh = m
			break
		}
	}
	pk.SetMovePP(0, 1)
	if err := Relearn(pk, Crystal, 1, fresh, nil); err != nil {
		t.Fatal(err)
	}
	if pk.Move(1) != fresh || pk.MovePP(1) != BasePP(2, fresh) || pk.MovePP(0) != 1 {
		t.Fatalf("slot1=%d pp=%d slot0pp=%d", pk.Move(1), pk.MovePP(1), pk.MovePP(0))
	}
	if err := Relearn(pk, Crystal, 2, fresh, nil); err == nil {
		t.Error("already known move must be refused")
	}
	if err := Relearn(pk, Crystal, 5, 1, nil); err == nil {
		t.Error("bad slot must be refused")
	}
	if err := Relearn(pk, Crystal, 2, 251, nil); err == nil {
		t.Error("move outside the pool must be refused")
	}
}

func TestBreeding(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	game := func(*Pokemon) Game { return Emerald }
	for _, g := range []Game{Crystal, Emerald} {
		sav := mustSave(t, g)
		sp := 2 // Ivysaur
		if g == Crystal {
			game = func(*Pokemon) Game { return Crystal }
		} else {
			sp = 2
		}
		if _, err := AddPokemon(sav, sp, 30, -1); err != nil {
			t.Fatal(err)
		}
		if _, err := AddPokemon(sav, speciesDitto, 30, -1); err != nil {
			t.Fatal(err)
		}
		a, _ := sav.Party(0)
		b, _ := sav.Party(1)
		if g == Emerald {
			for i := 0; i < 6; i++ {
				a.SetIV(i, 31)
				b.SetIV(i, 0)
			}
		}
		res, err := Breed(a, b, game, rng)
		if err != nil {
			t.Fatalf("%s: %v", g, err)
		}
		egg := res.Egg
		if !egg.IsEgg() || egg.Species() != 1 || egg.Level() != 5 || egg.Format() != sav.Generation() {
			t.Fatalf("%s: egg #%d L%d egg=%v", g, egg.Species(), egg.Level(), egg.IsEgg())
		}
		if egg.Friendship() == 0 || egg.Friendship() > 120 {
			t.Errorf("%s: hatch cycles = %d", g, egg.Friendship())
		}
		if egg.Move(0) == 0 {
			t.Errorf("%s: egg has no moves", g)
		}
		if g == Emerald {
			perfect := 0
			for i := 0; i < 6; i++ {
				if egg.IV(i) == 31 {
					perfect++
				}
			}
			if perfect < 3 {
				t.Errorf("only %d perfect IVs inherited/rolled", perfect)
			}
		}
		// The egg hatches after its steps are done.
		if err := Hatch(egg, g, LanguageEnglish); err == nil {
			t.Error("a fresh egg must not hatch")
		}
		egg.SetFriendship(0)
		if err := Hatch(egg, g, LanguageEnglish); err != nil {
			t.Fatalf("%s: hatch: %v", g, err)
		}
		if egg.IsEgg() || egg.Nickname() != defaultSpeciesNickname(1, LanguageEnglish, sav.Generation()) {
			t.Errorf("%s: hatched as %q egg=%v", g, egg.Nickname(), egg.IsEgg())
		}
		if egg.Friendship() == 0 {
			t.Errorf("%s: hatchling friendship not set", g)
		}
	}
	// Refusals.
	sav := mustSave(t, Crystal)
	mk := func(sp int) *Pokemon {
		pk, err := AddPokemon(sav, sp, 20, 1)
		if err != nil {
			t.Fatal(err)
		}
		return pk
	}
	gc := func(*Pokemon) Game { return Crystal }
	if _, err := Breed(mk(132), mk(132), gc, rng); err == nil || err.Error() != "Two Dittos cannot make an egg." {
		t.Errorf("two dittos: %v", err)
	}
	if _, err := Breed(mk(172), mk(1), gc, rng); err == nil { // Pichu is undiscovered
		t.Error("Pichu cannot breed")
	}
	if _, err := Breed(mk(1), mk(4), gc, rng); err == nil {
		t.Error("Bulbasaur and Charmander share no egg group")
	}
	if _, err := Breed(mk(144), mk(132), gc, rng); err == nil {
		t.Error("Articuno cannot breed")
	}
}

func TestBagLimits(t *testing.T) {
	for _, g := range Games() {
		sav := mustSave(t, g)
		for _, p := range sav.Pockets() {
			if len(p.Allowed) == 0 {
				continue
			}
			first := int(p.Allowed[len(p.Allowed)/2])
			if first == 0 {
				first = int(p.Allowed[1])
			}
			if err := sav.SetPocket(p.Index, []ItemStack{{Item: first, Count: p.MaxFor(first)}}); err != nil {
				t.Fatalf("%s %s: %v", g, p.Type, err)
			}
			if err := sav.SetPocket(p.Index, []ItemStack{{Item: first, Count: p.MaxFor(first) + 1}}); err == nil {
				t.Errorf("%s %s accepted an over-limit quantity", g, p.Type)
			}
			if err := sav.SetPocket(p.Index, []ItemStack{{Item: 0xFFFF, Count: 1}}); err == nil {
				t.Errorf("%s %s accepted an unknown item", g, p.Type)
			}
			if err := sav.SetPocket(p.Index, make([]ItemStack, p.Capacity+1)); err == nil {
				t.Errorf("%s %s accepted too many stacks", g, p.Type)
			}
			again := reopen(t, sav)
			var got []ItemStack
			for _, q := range again.Pockets() {
				if q.Index == p.Index {
					got = q.Entries
				}
			}
			if len(got) != 1 || got[0].Item != first {
				t.Errorf("%s %s round trip = %+v (want item %d)", g, p.Type, got, first)
			}
			sav.SetPocket(p.Index, nil)
		}
	}
}

func TestHMsLimitedToOne(t *testing.T) {
	sav := mustSave(t, Crystal)
	var tm Pocket
	for _, p := range sav.Pockets() {
		if p.Type == PocketTMHMs {
			tm = p
		}
	}
	if err := sav.SetPocket(tm.Index, []ItemStack{{Item: 243, Count: 2}}); err == nil {
		t.Error("two HMs should be refused")
	}
	if err := sav.SetPocket(tm.Index, []ItemStack{{Item: 243, Count: 1}, {Item: 191, Count: 50}}); err != nil {
		t.Error(err)
	}
}

func TestOpenErrorsAndFooters(t *testing.T) {
	if _, err := Open([]byte("not a save"), OpenOptions{}); err == nil {
		t.Error("garbage must be refused")
	}
	if _, err := Open(make([]byte, sizeG1Raw), OpenOptions{}); err == nil {
		t.Error("an all-zero GB save has no valid lists")
	}
	if _, err := Open(nil, OpenOptions{}); err == nil {
		t.Error("empty input must be refused")
	}
	for _, g := range []Game{RedBlue, Crystal, Emerald} {
		sav := mustSave(t, g)
		raw, _ := sav.Bytes()
		bad := append([]byte(nil), raw...)
		switch g.Generation() {
		case 1:
			bad[sav1Intl.checksum] ^= 0xFF
		case 2:
			bad[0x2D0D] ^= 0xFF
		default:
			bad[0x100] ^= 0xFF
		}
		if _, err := Open(bad, OpenOptions{}); err == nil || !strings.Contains(err.Error(), "checksums are invalid") {
			t.Errorf("%s: bad checksum error = %v", g, err)
		}
		if _, err := Open(bad, OpenOptions{AllowBadChecksums: true}); err != nil {
			t.Errorf("%s: AllowBadChecksums: %v", g, err)
		}
		// Emulator footers (RTC data) are preserved untouched.
		footer := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		withFooter := append(append([]byte(nil), raw...), footer...)
		opened, err := Open(withFooter, OpenOptions{})
		if err != nil {
			t.Errorf("%s: footer: %v", g, err)
			continue
		}
		out, _ := opened.Bytes()
		if !bytes.Equal(out, withFooter) {
			t.Errorf("%s: footer not preserved", g)
		}
	}
}

func TestSetGameFamilies(t *testing.T) {
	cases := []struct {
		save Game
		ok   []Game
		bad  []Game
	}{
		{RedBlue, []Game{Red, Blue, Green, RedBlue}, []Game{Yellow, Gold, Emerald}},
		{Yellow, []Game{Yellow}, []Game{Red, RedBlue}},
		{GoldSilver, []Game{Gold, Silver, GoldSilver}, []Game{Crystal, Red}},
		{Crystal, []Game{Crystal}, []Game{Gold}},
		{RubySapphire, []Game{Ruby, Sapphire, RubySapphire}, []Game{Emerald, FireRed}},
		{Emerald, []Game{Emerald}, []Game{Ruby}},
		{FireRed, []Game{FireRed, LeafGreen, FireRedLeafGreen}, []Game{Emerald, Ruby}},
	}
	for _, c := range cases {
		sav := mustSave(t, c.save)
		for _, g := range c.ok {
			if err := sav.SetGame(g); err != nil {
				t.Errorf("%s save should accept %s: %v", c.save, g, err)
			}
		}
		for _, g := range c.bad {
			if err := sav.SetGame(g); err == nil {
				t.Errorf("%s save should refuse %s", c.save, g)
			}
		}
	}
	if _, err := ParseGame("nope"); err == nil {
		t.Error("ParseGame accepted junk")
	}
	if g, err := ParseGame(" fr "); err != nil || g != FireRed {
		t.Errorf("ParseGame = %v %v", g, err)
	}
}

func TestPartyAndBoxEditing(t *testing.T) {
	for _, g := range []Game{Yellow, GoldSilver, FireRed} {
		sav := mustSave(t, g)
		for i := 0; i < 6; i++ {
			if _, err := AddPokemon(sav, 10+i, 10+i, -1); err != nil {
				t.Fatalf("%s: %v", g, err)
			}
		}
		if _, err := AddPokemon(sav, 16, 5, -1); err == nil {
			t.Errorf("%s: seventh party member accepted", g)
		}
		if err := sav.DeleteParty(1); err != nil || sav.PartyCount() != 5 {
			t.Fatalf("%s: delete: %v count=%d", g, err, sav.PartyCount())
		}
		// the species after the deleted slot slid down
		pk, _ := sav.Party(1)
		if pk.Species() != 12 {
			t.Errorf("%s: slot 1 is #%d after delete", g, pk.Species())
		}
		// fill a box
		for i := 0; i < sav.BoxSlotCount(); i++ {
			if _, err := AddPokemon(sav, 19, 5, 4); err != nil {
				t.Fatalf("%s: box fill %d: %v", g, i, err)
			}
		}
		if _, err := AddPokemon(sav, 19, 5, 4); err == nil || err.Error() != "This box is full or locked." {
			t.Errorf("%s: full box error = %v", g, err)
		}
		if err := sav.DeleteBox(4, 3); err != nil {
			t.Fatal(err)
		}
		if sav.FirstFreeBoxSlot(4) < 0 {
			t.Errorf("%s: deleting should free a slot", g)
		}
		again := reopen(t, sav)
		n := 0
		for s := 0; s < again.BoxSlotCount(); s++ {
			if b, _ := again.Box(4, s); b != nil {
				n++
			}
		}
		if n != again.BoxSlotCount()-1 {
			t.Errorf("%s: %d mons in box after reopen", g, n)
		}
	}
}

func TestPokemonFieldAccessors(t *testing.T) {
	for _, format := range []int{1, 2, 3} {
		pk := NewPokemon(format, false)
		pk.SetSpecies(25)
		pk.SetLevel(37)
		pk.SetTID(54321)
		pk.SetOTName("RED")
		pk.SetNickname("SPARKY")
		pk.SetMove(2, 85)
		pk.SetMovePPUps(2, 2)
		pk.HealPP()
		pk.SetEV(StatAttack, 200)
		pk.SetIV(StatSpeed, 9)
		if pk.Species() != 25 || pk.Level() != 37 || pk.TID() != 54321 || pk.OTName() != "RED" ||
			pk.Nickname() != "SPARKY" || pk.Move(2) != 85 || pk.MovePPUps(2) != 2 ||
			pk.EV(StatAttack) != 200 || pk.IV(StatSpeed) != 9 {
			t.Errorf("format %d accessors disagree: %d %d %d %q %q %d %d %d %d", format,
				pk.Species(), pk.Level(), pk.TID(), pk.OTName(), pk.Nickname(), pk.Move(2),
				pk.MovePPUps(2), pk.EV(StatAttack), pk.IV(StatSpeed))
		}
		if want := BasePP(format, 85) * 7 / 5; format == 3 && pk.MovePP(2) != want {
			t.Errorf("gen3 pp = %d, want %d", pk.MovePP(2), want)
		}
		if !pk.IsNicknamed() {
			t.Errorf("format %d: SPARKY should count as a nickname", format)
		}
		pk.SetNotNicknamed(LanguageEnglish)
		if pk.IsNicknamed() || pk.Nickname() != "PIKACHU" {
			t.Errorf("format %d: reset nickname = %q", format, pk.Nickname())
		}
		clone := pk.Clone()
		clone.SetLevel(50)
		if pk.Level() != 37 {
			t.Errorf("format %d: clone shares storage", format)
		}
		if format == 3 {
			pk.SetEgg(true)
			if !pk.IsEgg() || pk.Nickname() != "タマゴ" || pk.Language() != LanguageJapanese {
				t.Errorf("gen3 egg flags: egg=%v nick=%q lang=%v", pk.IsEgg(), pk.Nickname(), pk.Language())
			}
			pk.RefreshChecksum()
			if !pk.ChecksumValid() {
				t.Error("checksum not refreshed")
			}
		}
	}
}

func TestProfile(t *testing.T) {
	sav := mustSave(t, Emerald)
	pk, err := AddPokemon(sav, 252, 16, -1)
	if err != nil {
		t.Fatal(err)
	}
	p := ProfileOf(pk, Emerald)
	if p.SpeciesName != "Treecko" || p.Level != 16 || p.Nature == "" || p.Trainer != "GARDEN" || len(p.Moves) == 0 {
		t.Errorf("profile = %+v", p)
	}
	if p.IVs[0] != 15 || p.Stats[0] <= 0 {
		t.Errorf("ivs=%v stats=%v", p.IVs, p.Stats)
	}
}

func TestNewPokemonDefaults(t *testing.T) {
	sav := mustSave(t, Crystal)
	pk, err := AddPokemon(sav, 155, 20, -1)
	if err != nil {
		t.Fatal(err)
	}
	if pk.OTName() != "GARDEN" || pk.TID() != 1234 || pk.DV16() != 0x8888 || pk.HP() != pk.MaxHP() {
		t.Errorf("defaults: %q %d %x %d/%d", pk.OTName(), pk.TID(), pk.DV16(), pk.HP(), pk.MaxHP())
	}
	for _, m := range pk.Moves() {
		if m != 0 && pk.MovePP(0) == 0 {
			t.Error("moves must have PP")
		}
	}
	if _, err := AddPokemon(sav, 252, 5, -1); err == nil {
		t.Error("Treecko does not exist in Crystal")
	}
	if _, err := AddPokemon(sav, 1, 1, -1); err == nil {
		t.Error("level 1 is refused")
	}
}
