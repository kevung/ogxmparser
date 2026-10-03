package ogxmparser

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevung/ogxmparser/ogid"
)

// reference is what generate.py records from HedgeHog's reference
// implementation for one corpus file.
type reference struct {
	MatchLength int    `json:"match_length"`
	PlayerWhite string `json:"player_white"`
	PlayerBlack string `json:"player_black"`
	Games       []struct {
		Winner     *int `json:"winner"`
		PointsWon  *int `json:"points_won"`
		IsCrawford bool `json:"is_crawford"`
		Plies      []struct {
			ActionID     int      `json:"action_id"`
			OgidBefore   string   `json:"ogid_before"`
			OgidAfter    string   `json:"ogid_after"`
			Luck         *float64 `json:"luck"`
			Alternatives int      `json:"alternatives"`
		} `json:"plies"`
	} `json:"games"`
	Signatures []struct {
		Kind    string `json:"kind"`
		KeyID   string `json:"key_id"`
		Payload string `json:"payload"`
		Status  string `json:"status"`
	} `json:"signatures"`
}

func corpus(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("testdata/*.ogxm")
	if err != nil || len(files) == 0 {
		t.Fatalf("no corpus: %v", err)
	}
	return files
}

func loadRef(t *testing.T, path string) (*File, reference) {
	t.Helper()
	f, err := ParseFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	raw, err := os.ReadFile(strings.TrimSuffix(path, ".ogxm") + ".ref.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref reference
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	return f, ref
}

// TestReplayMatchesReference holds every reconstructed position to the OGID
// the reference implementation derives for the same ply.
func TestReplayMatchesReference(t *testing.T) {
	for _, path := range corpus(t) {
		f, ref := loadRef(t, path)
		if f.Match.Length != ref.MatchLength || f.Match.WhiteName != ref.PlayerWhite || f.Match.BlackName != ref.PlayerBlack {
			t.Errorf("%s: match %+v, reference %d %q %q", path, f.Match, ref.MatchLength, ref.PlayerWhite, ref.PlayerBlack)
		}
		games := f.Replay()
		if len(games) != len(ref.Games) {
			t.Fatalf("%s: %d games, reference %d", path, len(games), len(ref.Games))
		}
		for gi, g := range games {
			rg := ref.Games[gi]
			if g.FailedAt != -1 {
				t.Errorf("%s game %d: replay failed at %d: %v", path, gi, g.FailedAt, g.Err)
				continue
			}
			if g.Crawford != rg.IsCrawford {
				t.Errorf("%s game %d: crawford %v, reference %v", path, gi, g.Crawford, rg.IsCrawford)
			}
			if len(g.Plies) != len(rg.Plies) {
				t.Fatalf("%s game %d: %d plies, reference %d", path, gi, len(g.Plies), len(rg.Plies))
			}
			for pi, ps := range g.Plies {
				rp := rg.Plies[pi]
				if a := f.Games[gi].Plies[pi].Action; a != rp.ActionID {
					t.Errorf("%s game %d ply %d: action %d, reference %d", path, gi, pi, a, rp.ActionID)
				}
				if s := ps.Before.String(); s != rp.OgidBefore {
					t.Errorf("%s game %d ply %d before:\n got %s\nwant %s", path, gi, pi, s, rp.OgidBefore)
				}
				if s := ps.After.String(); s != rp.OgidAfter {
					t.Errorf("%s game %d ply %d after:\n got %s\nwant %s", path, gi, pi, s, rp.OgidAfter)
				}
				// The reference's OGIDs read back to themselves.
				for _, s := range []string{rp.OgidBefore, rp.OgidAfter} {
					o, err := ogid.Parse(s)
					if err != nil || o.String() != s {
						t.Errorf("ogid round trip %q: %q, %v", s, o.String(), err)
					}
				}
			}
		}
	}
}

// TestAnalysisMatchesReference checks the decoded analysis against the
// reference's projection: alternatives per checker decision, and luck.
func TestAnalysisMatchesReference(t *testing.T) {
	f, ref := loadRef(t, "testdata/hedgehog-3pt-analysed.ogxm")
	if len(f.Analyses) != 1 {
		t.Fatalf("%d analyses", len(f.Analyses))
	}
	a := f.Analyses[0]
	if a.ModelName != "xerxes" || a.METID != "rockwell-kazaross" || a.Currency != CubefulMoney {
		t.Errorf("analysis header: %q %q currency %d", a.ModelName, a.METID, a.Currency)
	}
	byRef := map[int]map[DecisionKind]Decision{}
	for _, d := range a.Decisions {
		if byRef[d.PlyRef] == nil {
			byRef[d.PlyRef] = map[DecisionKind]Decision{}
		}
		byRef[d.PlyRef][d.Kind] = d
	}
	ref0 := 0
	for gi, g := range ref.Games {
		for pi, rp := range g.Plies {
			ply := f.Games[gi].Plies[pi]
			if ply.Ref != ref0 {
				t.Fatalf("ply_ref %d, want %d", ply.Ref, ref0)
			}
			ds := byRef[ref0]
			ref0++
			n := 0
			if c, ok := ds[KindChecker]; ok {
				n = len(c.Checker.Alternatives)
			}
			if n != rp.Alternatives {
				t.Errorf("game %d ply %d: %d alternatives, reference %d", gi, pi, n, rp.Alternatives)
			}
			r, ok := ds[KindRoll]
			if (rp.Luck != nil) != ok || (ok && r.Roll.Luck != *rp.Luck) {
				t.Errorf("game %d ply %d: luck %v, reference %v", gi, pi, r.Roll, rp.Luck)
			}
		}
	}
}

// TestSignaturesMatchReference: the real file's SIGN and MSIG verify, over
// the very payloads the reference library builds.
func TestSignaturesMatchReference(t *testing.T) {
	for _, path := range corpus(t) {
		f, ref := loadRef(t, path)
		reports := f.VerifySignatures(nil)
		if len(reports) != len(ref.Signatures) {
			t.Fatalf("%s: %d signatures, reference %d", path, len(reports), len(ref.Signatures))
		}
		for i, r := range reports {
			if r.Status.String() != ref.Signatures[i].Status || r.KeyID != ref.Signatures[i].KeyID {
				t.Errorf("%s signature %d: %s %q, reference %s %q", path, i, r.Status, r.KeyID, ref.Signatures[i].Status, ref.Signatures[i].KeyID)
			}
		}
		signs, msigs := 0, 0
		for _, s := range ref.Signatures {
			want, _ := hex.DecodeString(s.Payload)
			var got []byte
			if s.Kind == "SIGN" {
				got = f.analysisPayload(&f.Analyses[signs])
				signs++
			} else {
				got, _ = f.matchPayload(&f.MatchSignatures[msigs])
				msigs++
			}
			if hex.EncodeToString(got) != hex.EncodeToString(want) {
				t.Errorf("%s %s payload differs from the reference", path, s.Kind)
			}
		}
	}
}

func TestTamperedSignatureIsInvalid(t *testing.T) {
	data, err := os.ReadFile("testdata/hedgehog-3pt-analysed.ogxm")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	f.Analyses[0].decsPayload = append([]byte{}, f.Analyses[0].decsPayload...)
	f.Analyses[0].decsPayload[len(f.Analyses[0].decsPayload)-1] ^= 1
	f.MatchSignatures[0].Signature = append([]byte{}, f.MatchSignatures[0].Signature...)
	f.MatchSignatures[0].Signature[0] ^= 1
	for _, r := range f.VerifySignatures(nil) {
		if r.Status != Invalid {
			t.Errorf("%q: %s after tampering", r.KeyID, r.Status)
		}
	}
	// Without an embedded key, a signature is unverifiable unless the caller
	// supplies one.
	pub := f.MatchSignatures[0].PublicKey
	f, _ = Parse(data)
	f.MatchSignatures[0].PublicKey = nil
	if r := f.VerifySignatures(nil); r[1].Status != Unverifiable {
		t.Errorf("no key: %s", r[1].Status)
	}
	lookup := func(id string) ed25519.PublicKey { return pub }
	if r := f.VerifySignatures(lookup); r[1].Status != Valid {
		t.Errorf("looked-up key: %s", r[1].Status)
	}
}

func TestCorruptionIsRejected(t *testing.T) {
	data, err := os.ReadFile("testdata/hedgehog-3pt-moves.ogxm")
	if err != nil {
		t.Fatal(err)
	}
	data = withCRC32(data)
	if f, err := Parse(data); err != nil || f.Checksum == nil || !f.Checksum.Verified {
		t.Fatalf("CSUM not verified: %v", err)
	}
	csumRecord := len(data) - 4 - 8
	for i := 0; i < len(data); i++ {
		if i == csumRecord+1 || i == csumRecord+2 {
			// An unknown mask bit or algorithm is skipped by design (I7, 8.5).
			continue
		}
		bad := append([]byte{}, data...)
		bad[i] ^= 0x10
		if _, err := Parse(bad); err == nil {
			t.Errorf("flipping byte %d went unnoticed", i)
		}
	}
	if _, err := Parse(data[:len(data)-1]); err == nil {
		t.Error("truncated file accepted")
	}
	if _, err := Parse([]byte("GNUBG")); err != ErrNotOGXM {
		t.Errorf("not OGXM: %v", err)
	}
}

func TestMatchFacts(t *testing.T) {
	f, err := ParseFile("testdata/hedgehog-3pt-analysed.ogxm")
	if err != nil {
		t.Fatal(err)
	}
	m := f.Match
	if m.Length != 3 || m.Rules&RuleCrawford == 0 || m.ScoreFinal == nil || *m.ScoreFinal != [2]int{2, 3} {
		t.Errorf("match facts: %+v", m)
	}
	if f.Shape() != ShapeCombined || f.Checksum != nil {
		t.Errorf("shape %d checksum %+v", f.Shape(), f.Checksum)
	}
	money, err := ParseFile("testdata/money-session.ogxm")
	if err != nil {
		t.Fatal(err)
	}
	g := money.Games[0]
	if g.Winner == nil || *g.Winner != White || *g.PointsWon != 16 {
		t.Errorf("resigned gammon on an 8 cube: %+v", g)
	}
	last := g.Plies[len(g.Plies)-1]
	if last.Action != ActionResignGame || last.ResignValue == nil || *last.ResignValue != 2 {
		t.Errorf("resign ply: %+v", last)
	}
	settle := money.Games[1].Plies[2]
	if settle.SettleValue == nil || *settle.SettleValue != -1.5 || !money.Games[1].IsLastGame {
		t.Errorf("settle ply: %+v", settle)
	}
}

// withCRC32 appends a CRC32 CSUM section (8.5) to a file that has none.
func withCRC32(data []byte) []byte {
	body := append([]byte{}, data[:len(data)-4]...)
	// CSUM record: length, empty mask, algorithm 0, 4-byte digest.
	size := len(body) + 9 + 8 + 4
	binary.LittleEndian.PutUint32(body[12:], uint32(size))
	body = append(body, 'C', 'S', 'U', 'M', 8, 0, 0, 0, 1)
	sum := crc32.ChecksumIEEE(body[:len(body)-9])
	rec := []byte{7, 0, 0, 4, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(rec[4:], sum)
	return append(append(body, rec...), 'E', 'N', 'D', '!')
}

func FuzzParse(f *testing.F) {
	for _, p := range []string{"testdata/hedgehog-3pt-moves.ogxm", "testdata/money-session.ogxm"} {
		if data, err := os.ReadFile(p); err == nil {
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := Parse(data)
		if err != nil {
			return
		}
		file.Replay()
		file.VerifySignatures(nil)
	})
}
