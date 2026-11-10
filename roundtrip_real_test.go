package pokeedit

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

// TestRealSave opens the developer's own Red save if present (skipped otherwise).
func TestRealSave(t *testing.T) {
	path := os.Getenv("POKEEDIT_REAL_SAVE")
	if path == "" {
		t.Skip("set POKEEDIT_REAL_SAVE to a .sav/.srm to run")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sav, err := Open(raw, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("game=%s gen=%d OT=%q TID=%d money=%d badges=%08b coins=%d\n", sav.Game(), sav.Generation(), sav.OT(), sav.TID(), sav.Money(), sav.Badges(), sav.Coins())
	for i := 0; i < sav.PartyCount(); i++ {
		pk, _ := sav.Party(i)
		fmt.Printf("  party %d: #%d %q lv%d hp %d/%d moves=%v ot=%q tid=%d\n", i, pk.Species(), pk.Nickname(), pk.Level(), pk.HP(), pk.MaxHP(), pk.Moves(), pk.OTName(), pk.TID())
	}
	for b := 0; b < sav.BoxCount(); b++ {
		n := 0
		for s := 0; s < sav.BoxSlotCount(); s++ {
			if pk, _ := sav.Box(b, s); pk != nil {
				n++
			}
		}
		if n > 0 {
			fmt.Printf("  box %d: %d mons\n", b, n)
		}
	}
	for _, p := range sav.Pockets() {
		fmt.Printf("  pocket %s: %v\n", p.Type, p.Entries)
	}
	seen, caught := 0, 0
	for i := 1; i <= sav.MaxSpecies(); i++ {
		if sav.Seen(i) {
			seen++
		}
		if sav.Caught(i) {
			caught++
		}
	}
	fmt.Println("dex seen/caught:", seen, caught)
	out, err := sav.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) {
		diff := 0
		first := -1
		for i := range raw {
			if i < len(out) && raw[i] != out[i] {
				if first < 0 {
					first = i
				}
				diff++
			}
		}
		t.Fatalf("round trip differs: %d bytes differ (first at 0x%X), len %d vs %d", diff, first, len(out), len(raw))
	}
}
