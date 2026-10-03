// Package ogxmparser reads OGXM, the OpenGammon eXtensible Match format of
// HedgeHog / OpenGammon: a backgammon match stored as the moves that were
// played, plus engine analyses of those moves, optionally signed with Ed25519.
//
// The format is specified in docs/OGXM_FORMAT_SPEC.md of HedgeHog's public
// repository (gitlab.com/eranlambooij/hedgehog-public), version 2.2, licensed
// CC BY 4.0. This package implements the reader side of that specification:
//
//   - Parse walks the container (2), every record (3.2) and section (4-8),
//     enforces the hard errors a reader must, and verifies what section 11
//     lists: file size, CSUM, score_final, a Combined file's match_digest.
//   - File.Replay reconstructs every position (5.4) and expresses each as the
//     OGID the reference implementation derives for it (package ogid).
//   - File.VerifySignatures reports each SIGN and MSIG as valid, invalid or
//     unverifiable (8.1, 8.6). A failed signature is never a load error, and
//     a key carried by the file identifies its signer, it does not make it
//     trustworthy.
//
// What it does not do: write files; read OGXM v1, or streams (2.6) other than
// one file at a time; check M4 (that a recorded play is legal for its dice;
// Board.Apply checks each step, M3); decode CLCK, VIDO and drawings, which
// are kept raw or counted.
package ogxmparser
