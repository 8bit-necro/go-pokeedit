package pokeedit_test

import (
	"fmt"
	"log"

	pe "github.com/8bit-necro/go-pokeedit"
)

// Open a save, add a Pokémon, rename it and write the result.
func Example() {
	// In real use: raw, _ := os.ReadFile("Pokemon Emerald.sav")
	blank, err := pe.NewSave(pe.Emerald, pe.NewSaveOptions{OT: "ANDY", TID: 4242})
	if err != nil {
		log.Fatal(err)
	}
	raw, _ := blank.Bytes()

	sav, err := pe.Open(raw, pe.OpenOptions{})
	if err != nil {
		log.Fatal(err) // unrecognized, damaged, or unsupported
	}
	pk, err := pe.AddPokemon(sav, 258 /* Mudkip */, 12, -1)
	if err != nil {
		log.Fatal(err)
	}
	pk, _ = sav.Party(0)
	pk.SetNickname("SPLASH")
	if err := sav.SetParty(0, pk); err != nil {
		log.Fatal(err)
	}
	sav.SetMoney(5000)

	out, err := sav.Bytes() // checksums and mirrored data are refreshed here
	if err != nil {
		log.Fatal(err)
	}
	again, _ := pe.Open(out, pe.OpenOptions{})
	got, _ := again.Party(0)
	fmt.Println(again.OT(), again.Money(), got.Nickname(), got.Level(), pe.MoveName(got.Move(0)))
	// Output: ANDY 5000 SPLASH 12 Tackle
}

// Move a Pokémon between saves of the same generation family, or between
// generations 1 and 2, via its portable byte encoding.
func ExamplePokemon_Bytes() {
	red, _ := pe.NewSave(pe.RedBlue, pe.NewSaveOptions{})
	gold, _ := pe.NewSave(pe.Gold, pe.NewSaveOptions{})
	pe.AddPokemon(red, 25, 20, -1)
	pe.AddPokemon(red, 4, 8, -1)

	pk, err := pe.Deposit(red, pe.PartySlot(0)) // leaves the party
	if err != nil {
		log.Fatal(err)
	}
	blob := pk.Bytes() // store anywhere: a database, a file, a network message

	restored, _ := pe.ParsePokemon(blob)
	if err := pe.Withdraw(gold, restored, -1); err != nil { // converts Gen 1 -> Gen 2
		log.Fatal(err)
	}
	moved, _ := gold.Party(0)
	fmt.Println(moved.Format(), pe.SpeciesName(moved.Species()), moved.Level())
	// Output: 2 Pikachu 20
}
