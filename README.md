# ogxmparser

Go package to parse OGXM match files and OGID position IDs (OpenGammon).

OGXM is the match format of [HedgeHog](https://hedgehog-bg.com) /
OpenGammon: the moves of a backgammon match, any number of engine analyses of
them (checker plays with ranked alternatives, cube decisions, luck), and
Ed25519 signatures over the match and over each analysis. OGID is its position
identifier, e.g. `11ccccchhhjjjjj:66666888dddddoo:N0N:46:B:IB:0:0:3:0`.

```go
f, err := ogxmparser.ParseFile("match.ogxm")
if err != nil { ... }
fmt.Println(f.Match.WhiteName, "vs", f.Match.BlackName, f.Match.Length)

for gi, g := range f.Replay() {          // positions, as OGIDs
    for pi, ply := range g.Plies {
        fmt.Println(gi, pi, ply.Before.String())
    }
}
for _, r := range f.VerifySignatures(nil) {
    fmt.Println(r.KeyID, r.Status)       // valid | invalid | unverifiable
}

o, err := ogid.Parse("cccccggggg:ddddiiiiii:N0N:63:W:IW:4:3:7:1:15")
fmt.Println(o.ToMove(), o.CubeValue(), o.MatchLength, o.Crawford)
```

## Specifications

Both formats are specified in HedgeHog's public repository,
[gitlab.com/eranlambooij/hedgehog-public](https://gitlab.com/eranlambooij/hedgehog-public):
`docs/OGXM_FORMAT_SPEC.md` (v2.2) and `docs/OGID.md`, licensed CC BY 4.0. This
package is an independent implementation written from them; where the prose
leaves a detail open it follows the reference implementation in the same
repository (MIT).

Implemented: the v2 container, records, every section's decoding, the hard
errors a reader enforces, the verified values of section 11 (file size, CSUM
CRC32 / SHA-256, `score_final`, `match_digest`), replay to positions (5.4) and
Ed25519 verification of `SIGN` and `MSIG`.

Not implemented: writing; OGXM v1 files and `.ogxms` streams; the M4 check
that a play is legal for its dice (each step is checked, M3); decoding of
`CLCK`, `VIDO` and annotation drawings (kept raw or counted).

## The OGID color field

The fifth field names the player who **reached** the position, so the other
one is to move: `...:46:B:IB:...` is White to play 4-6. `OGID.ToMove` applies
this, as the reference parser does.

## Tests and corpus

`testdata/` holds the corpus and, for each file, `.ref.json`: what the
reference implementation reads from it (every ply's OGID before and after,
analysis counts, luck, each signature's verdict and signing payload).
`testdata/generate.py` regenerates it from a build of the reference.

- `hedgehog-3pt-analysed.ogxm` is a match exported by HedgeHog with its
  analysis and its production `SIGN` and `MSIG`, taken from the test data of
  [AnkiGammon](https://github.com/Deinonychus999/AnkiGammon) (MIT).
- `hedgehog-3pt-moves.ogxm` and `money-session.ogxm` were written by the
  reference converter (`ogxm_convert --to-binary`) from the `.src.json` beside
  them; the money session exercises beaver, raccoon, a resignation, a
  last-game flag and a settlement.

```
go test ./...
go test -fuzz FuzzParse .
```

## License

MIT, see [LICENSE](LICENSE). The specifications this package implements are
© Eran Lambooij, CC BY 4.0; the corpus file taken from AnkiGammon is MIT, © 2025 AnkiGammon: its licence
is [testdata/LICENSE-AnkiGammon](testdata/LICENSE-AnkiGammon).
