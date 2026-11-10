// Package pokeedit reads and edits Pokémon main-series battery saves for the
// Game Boy, Game Boy Color and Game Boy Advance (generations 1–3): Red, Blue,
// Green, Yellow; Gold, Silver, Crystal; Ruby, Sapphire, Emerald, FireRed and
// LeafGreen.
//
// It is a Go port of the relevant parts of PKHeX (https://github.com/kwsch/PKHeX,
// GPL-3.0-or-later), using PKHeX's data tables, and is therefore licensed under
// GPL-3.0-or-later as well. The library never touches the filesystem: callers
// pass save bytes in and get save bytes out.
//
//	sav, err := pokeedit.Open(raw, pokeedit.OpenOptions{})
//	pk, _ := sav.Party(0)
//	pk.SetNickname("CHAMP")
//	_ = sav.SetParty(0, pk)
//	out, err := sav.Bytes()
package pokeedit

import (
	"fmt"
	"strings"
)

// Game identifies a specific main-series game, or a family of games that share
// a save layout and cannot be told apart from the save alone (RB, GS, RS, FRLG).
// The values match the short codes used by PKHeX and the pokegarden engine.
type Game string

// Games.
const (
	Red              Game = "RD"
	Blue             Game = "BU"
	Green            Game = "GN"
	RedBlue          Game = "RB"
	Yellow           Game = "YW"
	Gold             Game = "GD"
	Silver           Game = "SI"
	GoldSilver       Game = "GS"
	Crystal          Game = "C"
	Ruby             Game = "R"
	Sapphire         Game = "S"
	RubySapphire     Game = "RS"
	Emerald          Game = "E"
	FireRed          Game = "FR"
	LeafGreen        Game = "LG"
	FireRedLeafGreen Game = "FRLG"
)

var allGames = []Game{Red, Blue, Green, RedBlue, Yellow, Gold, Silver, GoldSilver, Crystal,
	Ruby, Sapphire, RubySapphire, Emerald, FireRed, LeafGreen, FireRedLeafGreen}

// Games returns every supported game code in a stable order.
func Games() []Game { return append([]Game(nil), allGames...) }

// ParseGame validates a game code such as "RB", "YW", "C" or "FRLG".
func ParseGame(s string) (Game, error) {
	g := Game(strings.ToUpper(strings.TrimSpace(s)))
	if g.Valid() {
		return g, nil
	}
	return "", fmt.Errorf("Unsupported GB/GBC/GBA game.")
}

// Valid reports whether g is a supported game code.
func (g Game) Valid() bool { return g.Generation() != 0 }

// Generation returns 1, 2 or 3, or 0 for an unknown game.
func (g Game) Generation() int {
	switch g {
	case Red, Blue, Green, RedBlue, Yellow:
		return 1
	case Gold, Silver, GoldSilver, Crystal:
		return 2
	case Ruby, Sapphire, RubySapphire, Emerald, FireRed, LeafGreen, FireRedLeafGreen:
		return 3
	}
	return 0
}

// String returns the short game code.
func (g Game) String() string { return string(g) }

// gen3Version is the numeric origin-game id stored in a generation 3 Pokémon.
func (g Game) gen3Version() int {
	switch g {
	case Sapphire:
		return 1
	case Ruby, RubySapphire:
		return 2
	case Emerald:
		return 3
	case FireRed, FireRedLeafGreen:
		return 4
	case LeafGreen:
		return 5
	}
	return 0
}

// Language is the in-game language id used by the games and PKHeX.
type Language int

// Languages.
const (
	LanguageNone     Language = 0
	LanguageJapanese Language = 1
	LanguageEnglish  Language = 2
	LanguageFrench   Language = 3
	LanguageItalian  Language = 4
	LanguageGerman   Language = 5
	LanguageSpanish  Language = 7
	LanguageKorean   Language = 8
)

// Species, move and item limits per generation.
const (
	maxSpecies1 = 151
	maxSpecies2 = 251
	maxSpecies3 = 386
	maxMove1    = 165
	maxMove2    = 251
	maxMove3    = 354
)

// MaxSpecies returns the highest national dex number of a generation.
func MaxSpecies(generation int) int {
	switch generation {
	case 1:
		return maxSpecies1
	case 2:
		return maxSpecies2
	}
	return maxSpecies3
}

// MaxMove returns the highest move id of a generation.
func MaxMove(generation int) int {
	switch generation {
	case 1:
		return maxMove1
	case 2:
		return maxMove2
	}
	return maxMove3
}

// Error is returned for every user-facing failure. Messages are written for end
// users and match the wording of the original pokegarden engine.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

func fail(format string, args ...any) error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}

func require(ok bool, msg string) error {
	if !ok {
		return &Error{Msg: msg}
	}
	return nil
}
