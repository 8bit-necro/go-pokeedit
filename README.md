# go-pokeedit

A Go library for reading and editing Pokémon main-series **battery saves** from the
Game Boy, Game Boy Color and Game Boy Advance.

I built this for [pokegarden](https://github.com/8bit-necro/pokegarden). Pokegarden was
leaning on [PKHeX](https://github.com/kwsch/PKHeX) through a small .NET bridge, which
worked great but meant shipping a .NET runtime and a C# build step just to poke at save
files. So I used PKHeX as the template and ported the parts pokegarden actually needs to
Go. It isn't a full PKHeX clone, only the slice I needed, kept as faithful to the original
as I could manage.

| Generation | Games | Notes |
| --- | --- | --- |
| 1 | Red, Blue, Green, Yellow | international and Japanese layouts |
| 2 | Gold, Silver, Crystal | international and Japanese layouts (not Korean) |
| 3 | Ruby, Sapphire, Emerald, FireRed, LeafGreen | rotating sectors, encrypted Pokémon |

It works on save **bytes** only and never touches the filesystem or the network. It's
pure Go, with no cgo and nothing outside the standard library.

**Out of scope:** generations 4–9, Colosseum/XD, Stadium, Korean saves, ROM hacks, emulator
save states, and PKHeX's legality checks. The library edits what is in a save and doesn't
judge whether it's "legal".

## Quick start

```go
raw, _ := os.ReadFile("Pokemon Emerald.sav")
sav, err := pokeedit.Open(raw, pokeedit.OpenOptions{}) // detects game, language and layout
if err != nil { log.Fatal(err) }                       // unrecognized / damaged / unsupported

pk, _ := pokeedit.AddPokemon(sav, 258 /* Mudkip */, 12, -1) // -1 = party, 0..n = box
pk, _ = sav.Party(0)
pk.SetNickname("SPLASH")
_ = sav.SetParty(0, pk)
_ = sav.SetMoney(5000)

out, _ := sav.Bytes() // checksums, mirrored copies and sector checksums refreshed
os.WriteFile("Pokemon Emerald.sav", out, 0o644)
```

## What's in here

### The `pokeedit` package (repo root)

| Piece | What it does |
| --- | --- |
| `Open`, `NewSave` | Detect and parse a save (handles emulator RTC footers, refuses damaged checksums unless told otherwise), or build a blank valid one |
| `Save` interface | Trainer (name, IDs, money, coins, badges, rival), party, boxes, Pokédex, bag pockets and game limits. `Save1`, `Save2` and `Save3` are the concrete types |
| `Pokemon` | One Pokémon in the Gen 1/2/3 layout, as a view over its raw bytes, so fields the library doesn't know about are preserved exactly. Covers species, nickname, OT, level/EXP, party stats, moves/PP/PP Ups, DVs/IVs, stat exp/EVs, held item, friendship, egg flag, nature, gender, shininess and met data |
| `Bytes` / `ParsePokemon` | A portable encoding of a Pokémon (the same `pkhex-v1` payload pokegarden stores), plus `ConvertFor` for Gen 1 ↔ 2 transfers |
| Operations | `AddPokemon`, `Deposit`, `Withdraw`, `HealPokemon`, `Recalculate`, `Hatch`, `Breed`, `EvolutionChoices`/`Evolve`, `MovePool`/`Relearn`, `ProfileOf`, `CompatibleGames` |
| Data | `PersonalFor`, `LearnsetOf`, `EvolutionsOf`, `ExpForLevel`/`LevelForExp`, `BasePP`, and species/move/item/nature names. These are PKHeX's own tables, embedded with `go:embed` |

### `engine` and `cmd/pokegarden-engine`

Pokegarden's old C# bridge spoke a simple JSON protocol: one request in, one response out.
`engine` implements that same protocol on top of the library, and `cmd/pokegarden-engine`
is the executable. The requests and responses are identical (`inspect`, `mutate`,
`deposit`, `withdraw`, `entity`, `profile`, `hatch`, `breed`, `training`, `compatible`,
`fixture`).

```sh
go build -o pokegarden-engine ./cmd/pokegarden-engine
echo '{"action":"fixture","game":"E"}' | ./pokegarden-engine
```

To switch pokegarden over, point `command()` in its `pokeedit/engine.py` at this binary
instead of `[dotnet, PokeGarden.Engine.dll]`. Nothing else on the Python side has to
change. A Go version of pokegarden could also import `engine` (or the root package)
directly and skip the process boundary entirely.

| | .NET bridge | Go |
| --- | --- | --- |
| Per request (spawn + inspect an Emerald save) | ~85 ms | ~5 ms |
| Cold `go build ./...` | n/a | ~1.7 s |
| Runtime needed | .NET 10 + PKHeX.Core | none (4.5 MB static binary) |

## How I checked it

I didn't want to just test the port against itself, so the checks run against the real PKHeX.Core:

* **Golden tests** (`go test ./...`): `testdata/pkhex/` holds saves generated through the PKHeX
  bridge for eight games, along with the `inspect` output it produced. The Go engine has to
  produce identical summaries and write each unmodified save back **byte for byte**.
* **Differential harness** (`tools/oracle/`, needs the .NET engine): sends identical requests to
  both engines and compares the responses and the resulting save bytes. The final run passed
  937/937 checks on `inspect` and every `mutate` scope (trainer, Pokédex, bag, add, heal, every
  Pokémon field) across all 16 game codes. It also covered deposit/withdraw (including Gen 1 ↔ 2
  transfers), entity, profile, training options, evolution, relearn, hatch and compatibility on
  every game, plus 87/87 breeding comparisons on everything that isn't random.
* **A real save**: opening and writing back a genuine Red `.srm` with no changes is byte-identical
  (`POKEEDIT_REAL_SAVE=path go test -run TestRealSave -v`).
* The Mankey stored in pokegarden's `garden.db` (legacy `core/ot/nickname` payload) decodes identically.

```sh
go build -o /tmp/go-engine ./cmd/pokegarden-engine
POKEGARDEN_DOTNET=dotnet \
POKEGARDEN_ENGINE_DLL=/path/to/PokeGarden.Engine.dll \
POKEEDIT_GO_ENGINE=/tmp/go-engine \
python3 tools/oracle/oracle.py        # or oracle2.py / breedcmp.py
```

## Where I deliberately differ from PKHeX

A few places where I chose not to copy PKHeX's output:

1. **Untouched bytes stay untouched.** PKHeX rewrites every unused list slot with zeros. The games
   leave the old bytes there, so this library does too, and opening then saving a file changes
   nothing you didn't edit. A slot a Pokémon is *removed from* is zeroed, though (PKHeX leaves
   recalculated junk stats there).
2. **Gen 1 ↔ 2 transfers keep current HP**, clamped to the new maximum. PKHeX drops it and the
   Pokémon arrives with 0 HP.
3. **`AddPokemon` fills in sensible defaults** for a freshly caught Pokémon: base friendship
   (Gen 2/3), Poké Ball and met level (Gen 3). PKHeX leaves these at zero.
4. **Hatched Gen 3 eggs get an English language flag** (or whichever one you pass to `Hatch`).
   PKHeX hatches them as Japanese because eggs carry a Japanese flag, whereas the games reset it
   on hatch.
5. **Editing a bag pocket also removes TMs you took out** of the Gen 2 TM pocket. PKHeX leaves
   stale counts behind.
6. **Pokémon serialization always writes a valid Gen 3 checksum.**

## Updating the data tables

`tables_gen.go` and `data/` are generated from the PKHeX source (character sets, species-id
remaps, PP, growth curves, item lists, personal/learnset/evolution tables and names). To
regenerate against another PKHeX release:

```sh
python3 tools/gen_tables.py /path/to/PKHeX-source .
gofmt -w tables_gen.go
go test ./...
```

## Caveats

* A `Save` is not safe for concurrent use, though the embedded data tables are.
* Gen 1/2 box and party lists are compact, so slot numbers shift when you remove a Pokémon
  (just like in the games).
* Saves with bad checksums are refused by default. Set `OpenOptions.AllowBadChecksums` to override.
* Close your emulator before writing a save back, and keep a backup.

## License

GPL-3.0-or-later (see `LICENSE`). The data tables and format knowledge come from
[PKHeX](https://github.com/kwsch/PKHeX) by Kaphotics and contributors (GPL-3.0-or-later). This
is a derivative work of it, as was the C# bridge it replaces.
