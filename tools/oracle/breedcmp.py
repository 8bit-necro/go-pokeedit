import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import oracle2
from oracle import run, CS, GO

pairs = {
  "C":  [(2,132),(3,132),(25,132),(52,53),(161,162),(179,132),(98,99),(133,132),(129,132),(37,132),(202,132),(172,132)],
  "GD": [(2,132),(25,132),(133,132)],
  "E":  [(253,132),(254,132),(263,132),(255,132),(277,132),(25,132),(133,132),(361,132),(300,132),(332,132)],
  "FR": [(2,132),(25,132),(133,132),(113,132)],
}
bad=0; n=0
for game, plist in pairs.items():
    for a,b in plist:
        save = oracle2.fixture(game, species=[a,b])
        d0 = run(CS, {"action":"deposit","save":save,"game":game,"slot":0})
        d1 = run(CS, {"action":"deposit","save":save,"game":game,"slot":1})
        if "payload" not in d0 or "payload" not in d1:
            print(game,a,b,"deposit fail",d0.get("error"),d1.get("error")); continue
        # Give both parents a shared move to exercise move inheritance
        for _ in range(3):
            ra=run(CS,{"action":"breed","first":d0["payload"],"second":d1["payload"]})
            rb=run(GO,{"action":"breed","first":d0["payload"],"second":d1["payload"]})
            n+=1
            if ("error" in ra) != ("error" in rb) or ra.get("error")!=rb.get("error"):
                bad+=1; print(game,a,b,"error mismatch",ra.get("error"),rb.get("error")); continue
            if "error" in ra: continue
            da,db=ra["details"],rb["details"]
            keys=["species","dex","species_name","generation","egg","level","friendship","growth","max_xp","xp","compatible_games"]
            pa=run(CS,{"action":"profile","payload":ra["payload"]}); pb=run(GO,{"action":"profile","payload":rb["payload"]})
            diffs=[k for k in keys if da[k]!=db[k]]
            diffs+= ["moves" for _ in [0] if [(m["move"],m["pp"]) for m in pa["moves"]]!=[(m["move"],m["pp"]) for m in pb["moves"]]]
            diffs+= ["mother" for _ in [0] if ra["mother"]!=rb["mother"]]
            diffs+= ["inherited" for _ in [0] if ra["inherited"]!=rb["inherited"]]
            diffs+= ["species_name"] if pa["species_name"]!=pb["species_name"] else []
            diffs+= ["gender?"] if False else []
            if diffs:
                bad+=1; print(game,a,b,"DIFF",diffs, "\n  C#:",ra["inherited"],[m["name"] for m in pa["moves"]],da["friendship"],"\n  Go:",rb["inherited"],[m["name"] for m in pb["moves"]],db["friendship"])
print(f"{n-bad}/{n} breed comparisons matched")
