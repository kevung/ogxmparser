#!/usr/bin/env python3
"""Regenerates the reference corpus from HedgeHog's reference implementation.

Requires a build of gitlab.com/eranlambooij/hedgehog-public (`make ogxm
NATIVE=0`) and the `cryptography` package:

    HEDGEHOG=/path/to/hedgehog-public python3 testdata/generate.py

For every testdata/*.ogxm it writes <name>.ref.json: what the reference
reads from the file (`ogxm_convert --to-json`), trimmed to the facts the Go
tests compare, plus each signature's verdict and signing payload as the
reference library computes them (python/ogxm/signature.py).

The synthetic sources (money-*.json) are converted to .ogxm by the reference
writer first, so every byte of the corpus was written or read by it.
"""

import base64
import glob
import json
import os
import subprocess
import sys

HEDGEHOG = os.environ.get("HEDGEHOG")
if not HEDGEHOG:
    sys.exit("set HEDGEHOG to a built hedgehog-public checkout")
CONVERT = os.path.join(HEDGEHOG, "build", "ogxm_convert")
os.environ.setdefault("LIBOGXM_PATH", os.path.join(HEDGEHOG, "build", "libogxm.so"))
sys.path.insert(0, os.path.join(HEDGEHOG, "python"))
from ogxm import signature  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))


def run(args, data):
    r = subprocess.run(args, input=data, capture_output=True, check=True)
    return r.stdout


def verdict(sig, payload):
    from cryptography.exceptions import InvalidSignature
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

    key = Ed25519PublicKey.from_public_bytes(base64.b64decode(sig["public_key"]))
    try:
        key.verify(base64.b64decode(sig["signature"]), payload)
        return "valid"
    except InvalidSignature:
        return "invalid"


for src in sorted(glob.glob(os.path.join(HERE, "*.src.json"))):
    out = src[: -len(".src.json")] + ".ogxm"
    with open(src, "rb") as f, open(out, "wb") as g:
        g.write(run([CONVERT, "--to-binary"], f.read()))

for path in sorted(glob.glob(os.path.join(HERE, "*.ogxm"))):
    data = open(path, "rb").read()
    doc = json.loads(run([CONVERT, "--to-json"], data))
    ref = {
        "match_length": doc.get("match_length"),
        "player_white": doc.get("player_white"),
        "player_black": doc.get("player_black"),
        "white_score": doc.get("white_score"),
        "black_score": doc.get("black_score"),
        "games": [],
        "signatures": [],
    }
    for g in doc["games"]:
        ref["games"].append({
            "winner": g.get("winner"),
            "points_won": g.get("points_won"),
            "is_crawford": g.get("is_crawford"),
            "plies": [
                {
                    "action_id": p["action_id"],
                    "ogid_before": p.get("ogid_before"),
                    "ogid_after": p.get("ogid_after"),
                    "luck": (p.get("analysis") or {}).get("roll", {}).get("luck"),
                    "alternatives": len((p.get("analysis") or {}).get("alternatives", [])),
                }
                for p in g["plies"]
            ],
        })
    info = doc.get("analysis_info")
    if info and info.get("signature"):
        payload = signature.analysis_payload(doc, 0)
        ref["signatures"].append({
            "kind": "SIGN", "key_id": info["signature"].get("key_id"),
            "payload": payload.hex(), "status": verdict(info["signature"], payload),
        })
    for i, s in enumerate(doc.get("match_signatures", [])):
        payload = signature.match_payload(doc, i)
        ref["signatures"].append({
            "kind": "MSIG", "key_id": s.get("key_id"),
            "payload": payload.hex(), "status": verdict(s, payload),
        })
    with open(path[: -len(".ogxm")] + ".ref.json", "w") as f:
        json.dump(ref, f, indent=1, sort_keys=True)
        f.write("\n")
    print(path, len(ref["games"]), "games,", len(ref["signatures"]), "signatures")
