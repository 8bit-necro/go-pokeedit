#!/usr/bin/env python3
"""Differential test: the PKHeX-based .NET engine (oracle) vs the Go engine.

Usage: oracle.py [GAME ...]   (default: every supported game code)
See README.md, "Verification".
"""
import base64
import json
import os
import subprocess
import sys

# Paths are configurable so the harness works from any checkout:
#   POKEGARDEN_ENGINE_DLL  PokeGarden.Engine.dll built with `dotnet build -c Release`
#   POKEGARDEN_DOTNET      the dotnet executable (default: `dotnet` on PATH)
#   POKEEDIT_GO_ENGINE     the Go engine binary (`go build ./cmd/pokegarden-engine`)
CS = [os.environ.get("POKEGARDEN_DOTNET", "dotnet"), os.environ["POKEGARDEN_ENGINE_DLL"]]
GO = [os.environ["POKEEDIT_GO_ENGINE"]]
ENV = {**os.environ, "DOTNET_NOLOGO": "1", "DOTNET_CLI_TELEMETRY_OPTOUT": "1"}

GAMES = ["RD", "BU", "GN", "RB", "YW", "GD", "SI", "GS", "C", "R", "S", "RS", "E", "FR", "LG", "FRLG"]


def run(cmd, req):
    p = subprocess.run(cmd, input=json.dumps(req), text=True, capture_output=True, env=ENV, timeout=60)
    try:
        return json.loads(p.stdout)
    except ValueError:
        return {"error": "NOT JSON: " + p.stdout[:200] + p.stderr[:200]}


def both(req):
    return run(CS, req), run(GO, req)


def diff(a, b, path=""):
    out = []
    if type(a) != type(b) and not (isinstance(a, (int, float)) and isinstance(b, (int, float))):
        return [f"{path}: type {type(a).__name__} != {type(b).__name__} ({str(a)[:60]} vs {str(b)[:60]})"]
    if isinstance(a, dict):
        for k in sorted(set(a) | set(b)):
            if k not in a:
                out.append(f"{path}/{k}: missing in C#")
            elif k not in b:
                out.append(f"{path}/{k}: missing in Go")
            else:
                out += diff(a[k], b[k], f"{path}/{k}")
    elif isinstance(a, list):
        if len(a) != len(b):
            out.append(f"{path}: len {len(a)} != {len(b)}")
        for i, (x, y) in enumerate(zip(a, b)):
            out += diff(x, y, f"{path}[{i}]")
    elif a != b:
        out.append(f"{path}: {str(a)[:80]!r} != {str(b)[:80]!r}")
    return out


FAILS = 0
CHECKS = 0


def check(name, a, b, limit=8):
    global FAILS, CHECKS
    CHECKS += 1
    d = diff(a, b)
    if d:
        FAILS += 1
        print(f"FAIL {name}")
        for line in d[:limit]:
            print("   ", line)
        if len(d) > limit:
            print(f"    ... {len(d) - limit} more")
    return not d


def byte_diff(a_b64, b_b64):
    a, b = base64.b64decode(a_b64), base64.b64decode(b_b64)
    if a == b:
        return []
    if len(a) != len(b):
        return [f"length {len(a)} != {len(b)}"]
    diffs = [i for i in range(len(a)) if a[i] != b[i]]
    return [f"{len(diffs)} bytes differ, first at 0x{diffs[0]:X}: {a[diffs[0]]:02x} vs {b[diffs[0]]:02x}"]


def compare_save(name, ra, rb):
    """Compare two engine responses that return a save."""
    global FAILS, CHECKS
    CHECKS += 1
    if "error" in ra or "error" in rb:
        if ra.get("error") != rb.get("error"):
            FAILS += 1
            print(f"FAIL {name}: error mismatch\n    C#: {ra.get('error')}\n    Go: {rb.get('error')}")
        return False
    d = byte_diff(ra["save"], rb["save"])
    if d:
        FAILS += 1
        print(f"FAIL {name}: save bytes {d[0]}")
        return False
    return True


def main():
    only = sys.argv[1:] or GAMES
    for game in only:
        print(f"== {game}")
        eggs = [False] + ([True] if game not in ("RD", "BU", "GN", "RB", "YW") else [])
        for egg in eggs:
            species = [1, 4, 7] if not game in ("C", "GD", "SI", "GS") else [152, 155, 158]
            if game in ("R", "S", "RS", "E", "FR", "LG", "FRLG"):
                species = [252, 255, 258]
            req = {"action": "fixture", "game": game, "species": species, "egg": egg}
            fx = run(CS, req)
            if "error" in fx:
                print("  fixture error:", fx["error"])
                continue
            save = fx["save"]
            tag = f"{game}{'+egg' if egg else ''}"
            ins = {"action": "inspect", "save": save, "game": game}
            check(f"{tag} inspect", *both(ins))
            # Go-built fixture must also be readable and equivalent in summary
            gfx = run(GO, req)
            if "error" in gfx:
                print("  go fixture error:", gfx["error"])
            else:
                ra = run(CS, {"action": "inspect", "save": gfx["save"], "game": game})
                rb = run(GO, {"action": "inspect", "save": gfx["save"], "game": game})
                check(f"{tag} inspect(go fixture)", ra, rb)
            # no-op round trip: mutate dex to current state yields same bytes via both
            yield_mutations(tag, save, game)
    print(f"\n{CHECKS - FAILS}/{CHECKS} checks passed")
    return 1 if FAILS else 0


def yield_mutations(tag, save, game):
    muts = []
    gen = 1 if game in ("RD", "BU", "GN", "RB", "YW") else 2 if game in ("GD", "SI", "GS", "C") else 3
    muts.append(("trainer OT", {"scope": "trainer", "field": "OT", "value": "ASH"}))
    muts.append(("trainer Money", {"scope": "trainer", "field": "Money", "value": 4321}))
    muts.append(("trainer Badges", {"scope": "trainer", "field": "Badges", "value": 5}))
    muts.append(("trainer TID", {"scope": "trainer", "field": "TID16", "value": 4242}))
    muts.append(("trainer SID", {"scope": "trainer", "field": "SID16", "value": 77}))
    muts.append(("trainer Coin", {"scope": "trainer", "field": "Coin", "value": 123}))
    muts.append(("trainer Gender", {"scope": "trainer", "field": "Gender", "value": 1}))
    muts.append(("trainer Rival", {"scope": "trainer", "field": "RivalName", "value": "GARY"}))
    muts.append(("dex", {"scope": "dex", "species": 10, "seen": True, "owned": True}))
    muts.append(("dex seen", {"scope": "dex", "species": 11, "seen": True, "owned": False}))
    muts.append(("heal", {"scope": "heal", "slot": 0}))
    muts.append(("Nickname", {"scope": "field", "slot": 0, "field": "Nickname", "value": "BOSS"}))
    muts.append(("Level", {"scope": "field", "slot": 0, "field": "CurrentLevel", "value": 37}))
    muts.append(("Species", {"scope": "field", "slot": 1, "field": "Species", "value": 5}))
    muts.append(("Move1", {"scope": "field", "slot": 0, "field": "Move1", "value": 85}))
    muts.append(("IV_ATK", {"scope": "field", "slot": 0, "field": "IV_ATK", "value": 7}))
    muts.append(("IV_HP", {"scope": "field", "slot": 0, "field": "IV_HP", "value": 7}))
    muts.append(("EV_HP", {"scope": "field", "slot": 0, "field": "EV_HP", "value": 100}))
    muts.append(("EV_SPD", {"scope": "field", "slot": 0, "field": "EV_SPD", "value": 100}))
    muts.append(("TID16 mon", {"scope": "field", "slot": 0, "field": "TID16", "value": 999}))
    muts.append(("HP mon", {"scope": "field", "slot": 0, "field": "Stat_HPCurrent", "value": 3}))
    muts.append(("box nick", {"scope": "field", "box": 0, "slot": 0, "field": "Nickname", "value": "BOXED"}))
    muts.append(("box level", {"scope": "field", "box": 0, "slot": 0, "field": "CurrentLevel", "value": 44}))
    muts.append(("box heal", {"scope": "heal", "box": 0, "slot": 0}))
    if gen >= 2:
        muts.append(("HeldItem", {"scope": "field", "slot": 0, "field": "HeldItem", "value": 3 if gen == 2 else 13}))
        muts.append(("Friendship", {"scope": "field", "slot": 0, "field": "CurrentFriendship", "value": 200}))
    if gen == 3:
        muts.append(("EV_ATK big", {"scope": "field", "slot": 0, "field": "EV_ATK", "value": 255}))
        muts.append(("EV total", {"scope": "field", "slot": 0, "field": "EV_DEF", "value": 255}))
        muts.append(("EV over", {"scope": "field", "slot": 0, "field": "EV_SPE", "value": 255}))
        muts.append(("IV_SPD", {"scope": "field", "slot": 0, "field": "IV_SPD", "value": 31}))
    # inventory: pick a pocket and an allowed item
    r = run(CS, {"action": "inspect", "save": save, "game": game})
    if "summary" not in r:
        print("  (skipping mutations: C# cannot open this fixture:", r.get("error"), ")")
        return
    info = r["summary"]
    for p in info["pockets"]:
        allowed = [int(k) for k in p["allowed"]]
        if not allowed:
            continue
        cand = [i for i in allowed if i > 0][:3]
        entries = [{"item": cand[0], "quantity": 5}] + ([{"item": cand[1], "quantity": 2}] if len(cand) > 1 else [])
        muts.append((f"inv {p['name']}", {"scope": "inventory", "pocket": p["index"], "entries": entries}))
    muts.append(("inv bad", {"scope": "inventory", "pocket": 0, "entries": [{"item": 99999, "quantity": 1}]}))
    muts.append(("add party", {"scope": "add", "species": 6 if gen != 3 else 255, "level": 40}))
    muts.append(("add box", {"scope": "add", "species": 9 if gen != 3 else 258, "level": 30, "box": 2}))
    for name, m in muts:
        req = {"action": "mutate", "save": save, "game": game, **m}
        ra, rb = both(req)
        if gen == 2 and m.get("scope") == "add" and "error" not in rb:
            # Go gives new Pokémon their species' base friendship (intentional);
            # normalize back to PKHeX's 0 before comparing bytes.
            slot = {"slot": 3} if "box" not in m else {"box": m["box"], "slot": 0}
            rb = run(GO, {"action": "mutate", "save": rb["save"], "game": game, "scope": "field",
                          "field": "CurrentFriendship", "value": 0, **slot})
        nondeterministic = gen == 3 and m.get("scope") == "add"
        if nondeterministic:
            check(f"{tag} mutate {name} (errors)", {"e": ra.get("error")}, {"e": rb.get("error")})
            continue
        if "error" in ra or "error" in rb:
            check(f"{tag} mutate {name}", {"e": ra.get("error")}, {"e": rb.get("error")})
        else:
            compare_save(f"{tag} mutate {name}", ra, rb)


if __name__ == "__main__":
    sys.exit(main())
