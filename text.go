package pokeedit

import (
	"embed"
	"strings"
	"sync"
)

//go:embed data/*
var dataFS embed.FS

// ---------------------------------------------------------------------------
// Character encodings

const (
	gbTerminator = 0x50
	gbTradeOT    = 0x5D
	g3Terminator = 0xFF
)

type clearMode int

const (
	clearNone clearMode = iota
	clearZero
	clear50
	clearFF
)

func gbTable(gen int, lang Language) *[256]rune {
	if gen == 1 {
		if lang == LanguageJapanese {
			return &charsG1JP
		}
		return &charsG1EN
	}
	switch lang {
	case LanguageJapanese:
		return &charsG2JP
	case LanguageFrench, LanguageGerman:
		return &charsG2FR
	case LanguageItalian, LanguageSpanish:
		return &charsG2IT
	}
	return &charsG2EN
}

// decodeGB decodes a generation 1/2 string. Decoding stops at the first byte
// without a printable mapping (including the 0x50 terminator).
func decodeGB(data []byte, gen int, lang Language) string {
	if len(data) == 0 {
		return ""
	}
	if data[0] == gbTradeOT {
		return "*"
	}
	table := gbTable(gen, lang)
	var sb strings.Builder
	for _, b := range data {
		c := table[b]
		if c == 0 {
			break
		}
		sb.WriteRune(c)
	}
	return sb.String()
}

func indexRune(table *[256]rune, c rune, skip rune) (byte, bool) {
	for i, r := range table {
		if r == c && r != skip {
			return byte(i), true
		}
	}
	return 0, false
}

// encodeGB writes value into dst as a generation 1/2 string and returns the
// number of bytes used (including the terminator).
func encodeGB(dst []byte, value string, maxLen int, gen int, lang Language, mode clearMode) int {
	switch mode {
	case clearZero:
		for i := range dst {
			dst[i] = 0
		}
	case clear50:
		for i := range dst {
			dst[i] = gbTerminator
		}
	}
	runes := []rune(value)
	if len(runes) == 0 {
		return 0
	}
	if runes[0] == '*' {
		if len(dst) > 1 {
			dst[0], dst[1] = gbTradeOT, gbTerminator
		}
		return 2
	}
	if len(runes) > maxLen {
		runes = runes[:maxLen]
	}
	table := gbTable(gen, lang)
	i := 0
	for ; i < len(runes) && i < len(dst); i++ {
		idx, ok := indexRune(table, runes[i], 0)
		if !ok {
			// べ ぺ へ り are stored as their katakana forms in the tables.
			if strings.ContainsRune("べぺへり", runes[i]) {
				idx, ok = indexRune(table, runes[i]+0x60, 0)
			}
			if !ok {
				break
			}
		}
		dst[i] = idx
	}
	if i == len(dst) {
		return i
	}
	dst[i] = gbTerminator
	return i + 1
}

func g3Table(lang Language) *[256]rune {
	if lang == LanguageJapanese {
		return &charsG3JP
	}
	return &charsG3EN
}

func g3QuoteLeft(lang Language) rune {
	switch lang {
	case LanguageEnglish, LanguageItalian, LanguageSpanish:
		return '“'
	case LanguageFrench:
		return '«'
	case LanguageGerman:
		return '„'
	}
	return '『'
}

func g3QuoteRight(lang Language) rune {
	switch lang {
	case LanguageEnglish, LanguageItalian, LanguageSpanish:
		return '”'
	case LanguageFrench:
		return '»'
	case LanguageGerman:
		return '“'
	}
	return '』'
}

// decodeG3 decodes a generation 3 string.
func decodeG3(data []byte, lang Language) string {
	table := g3Table(lang)
	var sb strings.Builder
	for _, b := range data {
		var c rune
		switch b {
		case 0xB1:
			c = g3QuoteLeft(lang)
		case 0xB2:
			c = g3QuoteRight(lang)
		default:
			c = table[b]
		}
		if c == g3Terminator {
			break
		}
		sb.WriteRune(c)
	}
	return sb.String()
}

func g3Remap(c rune, lang Language) (byte, bool) {
	switch c {
	case '’':
		return 0xB4, true
	case '“':
		if lang != LanguageGerman {
			return 0xB1, true
		}
		return 0xB2, true
	case '”', '»', '』':
		return 0xB2, true
	case '«', '„', '『':
		return 0xB1, true
	}
	return 0, false
}

// encodeG3 writes value into dst as a generation 3 string.
func encodeG3(dst []byte, value string, maxLen int, lang Language, mode clearMode) int {
	runes := []rune(value)
	if len(runes) > maxLen {
		runes = runes[:maxLen]
	}
	switch mode {
	case clearFF:
		for i := range dst {
			dst[i] = g3Terminator
		}
	case clearZero:
		for i := range dst {
			dst[i] = 0
		}
	}
	table := g3Table(lang)
	i := 0
	for ; i < len(runes) && i < len(dst); i++ {
		b, ok := byte(0), false
		if runes[i] != '“' {
			b, ok = indexRune(table, runes[i], g3Terminator)
		}
		if !ok {
			b, ok = g3Remap(runes[i], lang)
		}
		if !ok {
			break
		}
		dst[i] = b
	}
	if i < len(dst) {
		dst[i] = g3Terminator
		i++
	}
	return i
}

// ---------------------------------------------------------------------------
// Name lists

type nameLists struct {
	species map[Language][]string
	moves   []string
	natures []string
	types   []string
	items   [4][]string // index by generation
}

var (
	namesOnce sync.Once
	names     nameLists
)

func readLines(file string) []string {
	raw, err := dataFS.ReadFile("data/" + file)
	if err != nil {
		panic(err)
	}
	s := strings.TrimRight(string(raw), "\n")
	return strings.Split(s, "\n")
}

func loadNames() {
	namesOnce.Do(func() {
		names.species = map[Language][]string{}
		for lang, code := range map[Language]string{
			LanguageJapanese: "ja", LanguageEnglish: "en", LanguageFrench: "fr",
			LanguageItalian: "it", LanguageGerman: "de", LanguageSpanish: "es",
		} {
			names.species[lang] = readLines("species_" + code + ".txt")
		}
		names.moves = readLines("moves_en.txt")
		names.natures = readLines("natures_en.txt")
		names.types = readLines("types_en.txt")
		for g := 1; g <= 3; g++ {
			names.items[g] = readLines("items_g" + string(rune('0'+g)) + "_en.txt")
		}
	})
}

// SpeciesName returns the English species name for a national dex number
// ("" when out of range).
func SpeciesName(species int) string {
	loadNames()
	l := names.species[LanguageEnglish]
	if species < 1 || species >= len(l) {
		return ""
	}
	return l[species]
}

// MoveName returns the English move name ("" when out of range).
func MoveName(move int) string {
	loadNames()
	if move == 0 {
		return "(None)"
	}
	if move < 0 || move >= len(names.moves) {
		return ""
	}
	return names.moves[move]
}

// NatureName returns the English nature name for a generation 3 nature index.
func NatureName(nature int) string {
	loadNames()
	if nature < 0 || nature >= len(names.natures) {
		return ""
	}
	return names.natures[nature]
}

// TypeName returns the English name of a type id.
func TypeName(t int) string {
	loadNames()
	if t < 0 || t >= len(names.types) {
		return ""
	}
	return names.types[t]
}

// ItemName returns the English item name for an item id as stored by the given
// generation's games ("" when out of range).
func ItemName(generation, item int) string {
	loadNames()
	if generation < 1 || generation > 3 {
		return ""
	}
	l := names.items[generation]
	if item < 0 || item >= len(l) {
		return ""
	}
	return l[item]
}

// ItemNames returns the full English item name table of a generation.
func ItemNames(generation int) []string {
	loadNames()
	if generation < 1 || generation > 3 {
		return nil
	}
	return append([]string(nil), names.items[generation]...)
}

// defaultSpeciesNickname returns the name a game gives a freshly obtained
// Pokémon: upper case in generations 1–3, with the Mr. Mime period fix.
func defaultSpeciesNickname(species int, lang Language, generation int) string {
	loadNames()
	list := names.species[lang]
	if list == nil {
		list = names.species[LanguageEnglish]
	}
	if species == 0 {
		return eggName(lang, generation)
	}
	if species >= len(list) {
		return ""
	}
	nick := list[species]
	if lang == LanguageJapanese {
		return nick
	}
	nick = strings.ToUpper(nick)
	if lang == LanguageFrench {
		nick = stripDiacriticsFR(nick)
	}
	if generation >= 3 {
		if species == 83 && (lang == LanguageEnglish || lang == LanguageItalian || lang == LanguageSpanish) {
			r := []rune(nick)
			r[len(r)-2] = '\''
			nick = string(r)
		}
		return nick
	}
	if species == 122 { // Mr. Mime: "MR․MIME" (one-dot leader, no space)
		r := []rune(nick)
		for i, c := range r {
			if c == ' ' && i > 0 {
				r[i-1] = '․'
				r = append(r[:i], r[i+1:]...)
				break
			}
		}
		nick = string(r)
	}
	return nick
}

// DefaultNickname is the name a game gives a Pokémon of a species that has not
// been nicknamed, in a language and generation.
func DefaultNickname(species int, lang Language, generation int) string {
	return defaultSpeciesNickname(species, lang, generation)
}

func stripDiacriticsFR(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'É', 'È', 'Ê', 'Ë':
			return 'E'
		case 'Î', 'Ï':
			return 'I'
		}
		return r
	}, s)
}

func eggName(lang Language, generation int) string {
	if generation == 3 {
		return "タマゴ"
	}
	if lang == LanguageFrench {
		return "OEUF"
	}
	loadNames()
	list := names.species[lang]
	if list == nil {
		list = names.species[LanguageEnglish]
	}
	return strings.ToUpper(list[0])
}

// guessLanguage finds which language's default nickname matches name.
func guessLanguage(species int, name string, generation int) Language {
	for _, lang := range []Language{LanguageEnglish, LanguageFrench, LanguageItalian, LanguageGerman, LanguageSpanish, LanguageJapanese} {
		if defaultSpeciesNickname(species, lang, generation) == name {
			return lang
		}
	}
	return LanguageNone
}

func isGermanGlyph(data []byte) bool {
	for _, b := range data {
		if b-0xC0 <= 6 {
			return true
		}
	}
	return false
}
