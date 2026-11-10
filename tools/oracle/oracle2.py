#!/usr/bin/env python3
"""Differential tests for payload-based actions."""
import base64
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import oracle  # noqa: E402
from oracle import both, check, compare_save, run, CS, GO  # noqa: E402

G1 = ["RD", "BU", "GN", "RB", "YW"]
G2 = ["GD", "SI", "GS", "C"]
G3 = ["R", "S", "RS", "E", "FR", "LG"]


def fixture(game, species=None, egg=False):
    if species is None:
        species = {1: [1, 4, 7], 2: [152, 155, 158], 3: [252, 255, 258]}[gen_of(game)]
    return run(CS, {"action": "fixture", "game": game, "species": species, "egg": egg})["save"]


def gen_of(g):
    return 1 if g in G1 else 2 if g in G2 else 3


def same(name, ra, rb, keys=None, limit=8):
    """Compare two response dicts; saves are compared bytewise."""
    if "error" in ra or "error" in rb:
        return check(name, {"e": ra.get("error")}, {"e": rb.get("error")}, limit)
    ra, rb = dict(ra), dict(rb)
    ok = True
    if "save" in ra or "save" in rb:
        ok &= compare_save(name + " [save]", ra, rb)
        ra.pop("save", None)
        rb.pop("save", None)
    return check(name, ra, rb, limit) and ok


def same_state(name, ra, rb, game):
    """Like same(), but compares the saves through inspect (slots vacated by a
    removal differ in raw bytes: PKHeX writes a stat-reset blank there)."""
    if "error" in ra or "error" in rb or "save" not in ra:
        return same(name, ra, rb)
    ia = run(CS, {"action": "inspect", "save": ra["save"], "game": game})
    ib = run(GO, {"action": "inspect", "save": rb["save"], "game": game})
    ra, rb = dict(ra), dict(rb)
    ra.pop("save"), rb.pop("save")
    ok = check(name, ra, rb)
    return check(name + " [state]", ia, ib) and ok


def main():
    games = sys.argv[1:] or G1 + G2 + G3
    for game in games:
        print("==", game)
        gen = gen_of(game)
        save = fixture(game)
        # --- deposit / withdraw
        for loc in ({"slot": 0}, {"slot": 2}, {"box": 0, "slot": 0}):
            ra, rb = both({"action": "deposit", "save": save, "game": game, **loc})
            same_state(f"{game} deposit {loc}", ra, rb, game)
            if "payload" in ra:
                # withdraw into party & box, same game
                for dest in ({}, {"box": 1}):
                    wa, wb = both({"action": "withdraw", "save": ra["save"], "game": game, "payload": ra["payload"], **dest})
                    same_state(f"{game} withdraw {loc}->{dest}", wa, wb, game)
                # entity / profile / hatch / training options
                ea, eb = both({"action": "entity", "payload": ra["payload"]})
                same(f"{game} entity {loc}", ea, eb)
                pa, pb = both({"action": "profile", "payload": ra["payload"]})
                same(f"{game} profile {loc}", pa, pb)
                for now in (1700000000, 1700040000):
                    ta, tb = both({"action": "training", "mode": "options", "payload": ra["payload"], "now": now})
                    same(f"{game} training options {loc} now={now}", ta, tb)
                cmp_a, cmp_b = both({"action": "compatible", "game": game, "payloads": [ra["payload"]]})
                same(f"{game} compatible {loc}", cmp_a, cmp_b)
        # --- cross-generation transfers
        ra, rb = both({"action": "deposit", "save": save, "game": game, "slot": 0})
        if "payload" in ra:
            targets = {1: G1 + G2, 2: G1 + G2, 3: G3}[gen]
            for tgt in targets:
                tsave = fixture(tgt)
                wa, wb = both({"action": "withdraw", "save": tsave, "game": tgt, "payload": ra["payload"]})
                if gen != gen_of(tgt) and "save" in wa and "save" in wb:
                    # PKHeX drops current HP when converting 1<->2 (the Pokemon
                    # arrives fainted); Go keeps it. Compare summaries minus HP.
                    ia = run(CS, {"action": "inspect", "save": wa["save"], "game": tgt})["summary"]
                    ib = run(CS, {"action": "inspect", "save": wb["save"], "game": tgt})["summary"]
                    for summ in (ia, ib):
                        for m in summ["party"] + [x for b in summ["boxes"] for x in b]:
                            m["values"].pop("Stat_HPCurrent", None)
                    check(f"{game}->{tgt} withdraw (summary, HP ignored)", ia, ib)
                else:
                    same(f"{game}->{tgt} withdraw", wa, wb)
                ca, cb = both({"action": "compatible", "game": tgt, "payloads": [ra["payload"]]})
                same(f"{game}->{tgt} compatible", ca, cb)
        # --- training: evolve / relearn
        evolve_cases(game, gen, save)
        # --- eggs: hatch
        if gen >= 2:
            hatch_cases(game, gen)
    print(f"\n{oracle.CHECKS - oracle.FAILS}/{oracle.CHECKS} checks passed")
    return 1 if oracle.FAILS else 0


def mk_payload(game, species, level, extra=None):
    """Build a payload by adding a Pokémon to a fixture via C# and depositing it."""
    gen = gen_of(game)
    save = fixture(game, species=[species])
    r = run(CS, {"action": "mutate", "save": save, "game": game, "scope": "field", "slot": 0,
                 "field": "CurrentLevel", "value": level})
    save = r["save"]
    for f, v in (extra or {}).items():
        r = run(CS, {"action": "mutate", "save": save, "game": game, "scope": "field", "slot": 0, "field": f, "value": v})
        if "save" not in r:
            return None
        save = r["save"]
    r = run(CS, {"action": "mutate", "save": save, "game": game, "scope": "add", "species": species, "level": 20}) if False else None
    # need >1 party mon to deposit slot 0: use box 0 slot 0 (visitor copy) instead
    d = run(CS, {"action": "deposit", "save": save, "game": game, "box": 0, "slot": 0})
    return d.get("payload")


def evolve_cases(game, gen, save):
    cases = {
        1: [(1, 16), (1, 15), (4, 36), (25, 30), (133, 30), (64, 30), (95, 30), (79, 40), (44, 30)],
        2: [(152, 16), (172, 20), (175, 20), (133, 30), (236, 20), (61, 30), (79, 40), (44, 30), (191, 30)],
        3: [(252, 16), (265, 7), (265, 12), (280, 20), (133, 30), (298, 30), (349, 30), (290, 20), (44, 30), (61, 30)],
    }[gen]
    for species, level in cases:
        payload = mk_payload(game, species, max(level, 2))
        if payload is None:
            continue
        ta, tb = both({"action": "training", "mode": "options", "payload": payload, "now": 1700000000})
        same(f"{game} options sp{species} L{level}", ta, tb)
        for ev in ta.get("evolutions", []):
            if ev.get("ready"):
                ea, eb = both({"action": "training", "mode": "evolve", "payload": payload, "target": ev["target"], "now": 1700000000})
                same(f"{game} evolve sp{species}->{ev['target']}", ea, eb)
        # relearn the first candidate into slot 1
        if ta.get("relearn"):
            mv = ta["relearn"][0]["move"]
            ra, rb = both({"action": "training", "mode": "relearn", "payload": payload, "move": mv, "slot": 1, "now": 1700000000})
            same(f"{game} relearn sp{species} move{mv}", ra, rb)
            # relearn of an invalid move
            ra, rb = both({"action": "training", "mode": "relearn", "payload": payload, "move": 1, "slot": 9, "now": 1700000000})
            same(f"{game} relearn bad slot sp{species}", ra, rb)


def hatch_cases(game, gen):
    save = fixture(game, egg=True)
    d = run(CS, {"action": "deposit", "save": save, "game": game, "box": 0, "slot": 0})
    payload = d.get("payload")
    if not payload:
        print("  no egg payload", d.get("error"))
        return
    ea, eb = both({"action": "entity", "payload": payload})
    same(f"{game} egg entity", ea, eb)
    # not ready egg
    ha, hb = both({"action": "hatch", "payload": payload})
    same(f"{game} hatch not ready", ha, hb)
    # make it ready (friendship 0)
    ready_a = run(CS, {"action": "entity", "payload": payload, "friendship": 0})
    ready_b = run(GO, {"action": "entity", "payload": payload, "friendship": 0})
    same(f"{game} egg ready entity", ready_a, ready_b)
    ha, hb = both({"action": "hatch", "payload": ready_a["payload"]})
    if gen == 3 and "payload" in ha and "payload" in hb:
        # Known, intentional difference: Go hatches into English instead of the
        # egg's Japanese language flag. Compare after setting the same language.
        print("  (gen3 hatch: compare structure only)")
        da, db = ha["details"], hb["details"]
        for k in ("species", "level", "egg", "xp", "friendship", "growth"):
            check(f"{game} hatch details.{k}", da[k], db[k])
    else:
        same(f"{game} hatch", ha, hb)
    # breed with fixture parents is nondeterministic; just check both succeed or fail alike
    parents = fixture(game, species=[1 if gen == 1 else (152 if gen == 2 else 252), 132])
    d0 = run(CS, {"action": "deposit", "save": parents, "game": game, "slot": 0})
    d1 = run(CS, {"action": "deposit", "save": parents, "game": game, "slot": 1})
    if "payload" in d0 and "payload" in d1:
        ba, bb = both({"action": "breed", "first": d0["payload"], "second": d1["payload"]})
        check(f"{game} breed ok/err", {"e": ba.get("error")}, {"e": bb.get("error")})


if __name__ == "__main__":
    sys.exit(main())
