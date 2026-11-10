package pokeedit

import (
	"bytes"
	"testing"
)

func mustSave(t *testing.T, g Game) Save {
	t.Helper()
	s, err := NewSave(g, NewSaveOptions{})
	if err != nil {
		t.Fatalf("%s: NewSave: %v", g, err)
	}
	return s
}

func reopen(t *testing.T, s Save) Save {
	t.Helper()
	raw, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out, err := Open(raw, OpenOptions{Game: s.Game()})
	if err != nil {
		t.Fatalf("reopen %s: %v", s.Game(), err)
	}
	return out
}

func TestNewSaveRoundTrip(t *testing.T) {
	for _, g := range Games() {
		t.Run(string(g), func(t *testing.T) {
			s := mustSave(t, g)
			species := []int{1, 4, 7, 25}
			for _, sp := range species {
				if sp > s.MaxSpecies() {
					continue
				}
				if _, err := AddPokemon(s, sp, 5+sp%10, -1); err != nil && s.PartyCount() < 6 {
					t.Fatalf("add %d: %v", sp, err)
				}
			}
			if _, err := AddPokemon(s, 25, 12, 0); err != nil {
				t.Fatalf("add to box: %v", err)
			}
			if err := s.SetMoney(123456); err != nil {
				t.Fatal(err)
			}
			again := reopen(t, s)
			if again.OT() != "GARDEN" || again.TID() != 1234 {
				t.Fatalf("trainer lost: %q %d", again.OT(), again.TID())
			}
			if again.Money() != 123456 {
				t.Fatalf("money = %d", again.Money())
			}
			if again.PartyCount() != s.PartyCount() {
				t.Fatalf("party %d vs %d", again.PartyCount(), s.PartyCount())
			}
			for i := 0; i < s.PartyCount(); i++ {
				a, _ := s.Party(i)
				b, _ := again.Party(i)
				if !bytes.Equal(a.Bytes(), b.Bytes()) {
					t.Fatalf("party %d differs\n%x\n%x", i, a.Bytes(), b.Bytes())
				}
				if a.Nickname() != SpeciesNameGen(a.Species()) {
					t.Logf("nick %q", a.Nickname())
				}
			}
			bx, _ := again.Box(0, 0)
			if bx == nil || bx.Species() != 25 {
				t.Fatalf("box mon lost: %v", bx)
			}
			if !again.Caught(25) || !again.Seen(25) {
				t.Fatalf("dex not updated")
			}
			// A second write of the reopened save must be stable.
			r1, _ := again.Bytes()
			r2, _ := reopen(t, again).Bytes()
			if !bytes.Equal(r1, r2) {
				t.Fatalf("not idempotent")
			}
		})
	}
}

// SpeciesNameGen is only used by the test above for diagnostics.
func SpeciesNameGen(species int) string { return defaultSpeciesNickname(species, LanguageEnglish, 3) }
