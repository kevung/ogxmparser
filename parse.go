package ogxmparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"time"
)

// ReaderMajor and ReaderMinor are the format version this reader implements.
const (
	ReaderMajor = 2
	ReaderMinor = 2
)

// ErrNotOGXM is returned when the data does not start with the OGXM magic.
var ErrNotOGXM = errors.New("ogxm: not an OGXM file")

// ErrVersion is returned for a file this reader is too old to read, or for
// a v1 file, which this reader does not convert.
var ErrVersion = errors.New("ogxm: unsupported version")

// ParseFile reads and parses one OGXM file from disk.
func ParseFile(path string) (*File, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	if st, err := fh.Stat(); err == nil && st.Size() > MaxFileSize {
		return nil, &FormatError{Msg: "file larger than MAX_FILE_SIZE"}
	}
	return ParseReader(fh)
}

// ParseReader reads r to the end and parses it as one OGXM file.
func ParseReader(r io.Reader) (*File, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileSize {
		return nil, &FormatError{Msg: "file larger than MAX_FILE_SIZE"}
	}
	return Parse(data)
}

// IsOGXM reports whether data starts with the OGXM magic.
func IsOGXM(data []byte) bool { return len(data) >= 4 && string(data[:4]) == "OGXM" }

// Parse parses one OGXM v2 file. It verifies everything section 11 lists as
// verified (file size, CSUM, score_final, a Combined file's match_digest,
// record walks) and the replay rules this reader implements (Replay); it does
// not verify signatures, which VerifySignatures reports on.
func Parse(data []byte) (*File, error) {
	if !IsOGXM(data) {
		return nil, ErrNotOGXM
	}
	if len(data) < 20 {
		return nil, &FormatError{Msg: "file shorter than header and end marker"}
	}
	h := cursor{b: data, off: 4}
	f := &File{}
	f.VersionMajor, _ = h.u16()
	f.VersionMinor, _ = h.u16()
	f.MinReaderMajor, _ = h.u16()
	f.MinReaderMinor, _ = h.u16()
	size, _ := h.u32()
	if f.VersionMajor < 2 {
		return nil, ErrVersion
	}
	if f.MinReaderMajor > ReaderMajor || (f.MinReaderMajor == ReaderMajor && f.MinReaderMinor > ReaderMinor) {
		return nil, ErrVersion
	}
	if size > MaxFileSize || int(size) != len(data) {
		return nil, &FormatError{Offset: 12, Msg: "file_size does not match the data"}
	}
	if string(data[len(data)-4:]) != "END!" {
		return nil, &FormatError{Offset: len(data) - 4, Msg: "missing END! marker"}
	}

	p := parser{f: f, data: data}
	if err := p.sections(); err != nil {
		return nil, err
	}
	if err := p.check(); err != nil {
		return nil, err
	}
	return f, nil
}

type parser struct {
	f    *File
	data []byte

	stage       int // position in the order of 2.4
	sawCritical bool
	plyCount    int
	pending     *Analysis // ANAL waiting for its DECS
	analysisIDs map[[16]byte]bool
}

const (
	stageStart = iota
	stageMtch
	stageGames
	stageAnalyses
	stageAncillary
	stageMsig
	stageCsum
)

func (p *parser) sections() error {
	off := 16
	end := len(p.data) - 4
	count := 0
	for off < end {
		count++
		if count > MaxSections {
			return &FormatError{Offset: off, Msg: "too many sections"}
		}
		if end-off < 9 {
			return &FormatError{Offset: off, Msg: "truncated section header"}
		}
		typ := string(p.data[off : off+4])
		length := binary.LittleEndian.Uint32(p.data[off+4:])
		flags := p.data[off+8]
		if flags&^1 != 0 {
			return &FormatError{Section: typ, Msg: "reserved section flag bits set"}
		}
		if length > MaxSectionSize || int(length) > end-off-9 {
			return &FormatError{Section: typ, Msg: "section length out of bounds"}
		}
		payload := p.data[off+9 : off+9+int(length)]
		if err := p.section(typ, flags&1 != 0, payload, off); err != nil {
			var fe *FormatError
			if errors.As(err, &fe) && fe.Section == "" {
				fe.Section = typ
			}
			return err
		}
		off += 9 + int(length)
	}
	if p.pending != nil {
		return &FormatError{Section: "ANAL", Msg: "ANAL not followed by its DECS"}
	}
	return nil
}

func (p *parser) order(stage int, typ string) error {
	if stage < p.stage {
		return &FormatError{Section: typ, Msg: "section out of order"}
	}
	if p.pending != nil && typ != "DECS" {
		return &FormatError{Section: typ, Msg: "ANAL not followed by its DECS"}
	}
	p.stage = stage
	return nil
}

func (p *parser) section(typ string, critical bool, payload []byte, headerOff int) error {
	f := p.f
	wantCritical := map[string]bool{"MTCH": true, "GAME": true, "CSUM": true}
	switch typ {
	case "MTCH", "GAME", "ANAL", "DECS", "SIGN", "CLCK", "VIDO", "ANNO", "MSIG", "CSUM":
		// ANAL and DECS are critical exactly in Analysis shape, which is only
		// known at the end; check() verifies them against the shape.
		if typ != "ANAL" && typ != "DECS" && critical != wantCritical[typ] {
			return &FormatError{Section: typ, Msg: "wrong CRITICAL bit"}
		}
	default:
		if critical {
			return &FormatError{Section: typ, Msg: "unknown critical section"}
		}
		return nil // unknown ancillary section: skipped (I7)
	}
	if critical {
		p.sawCritical = true
	}
	c := cursor{b: payload}
	switch typ {
	case "MTCH":
		if p.stage >= stageMtch {
			return &FormatError{Section: typ, Msg: "MTCH out of order or repeated"}
		}
		p.stage = stageMtch
		m, err := decodeMatch(&c)
		if err != nil {
			return err
		}
		f.Match = m
		f.mtchPayload = payload
	case "GAME":
		if f.Match == nil {
			return &FormatError{Section: typ, Msg: "GAME without MTCH"}
		}
		if err := p.order(stageGames, typ); err != nil {
			return err
		}
		if len(f.Games) >= MaxGames {
			return &FormatError{Section: typ, Msg: "too many games"}
		}
		g, err := p.decodeGame(&c)
		if err != nil {
			return err
		}
		f.Games = append(f.Games, *g)
		f.gamePayloads = append(f.gamePayloads, payload)
	case "ANAL":
		if err := p.order(stageAnalyses, typ); err != nil {
			return err
		}
		if len(f.Analyses) >= MaxAnalyses {
			return &FormatError{Section: typ, Msg: "too many analyses"}
		}
		a, err := decodeAnalysis(&c)
		if err != nil {
			return err
		}
		if p.analysisIDs == nil {
			p.analysisIDs = map[[16]byte]bool{}
		}
		if p.analysisIDs[a.ID] {
			return &FormatError{Section: typ, Msg: "duplicate analysis_id"}
		}
		p.analysisIDs[a.ID] = true
		a.analPayload = payload
		a.critical = critical
		p.pending = a
	case "DECS":
		if p.pending == nil {
			return &FormatError{Section: typ, Msg: "DECS without ANAL"}
		}
		a := p.pending
		p.pending = nil
		if critical != a.critical {
			return &FormatError{Section: typ, Msg: "DECS and ANAL CRITICAL bits differ"}
		}
		if err := p.decodeDecisions(&c, a); err != nil {
			return err
		}
		a.decsPayload = payload
		f.Analyses = append(f.Analyses, *a)
	case "SIGN":
		if p.stage != stageAnalyses || len(f.Analyses) == 0 || p.pending != nil {
			return &FormatError{Section: typ, Msg: "SIGN without its analysis block"}
		}
		last := &f.Analyses[len(f.Analyses)-1]
		if last.Signature != nil {
			return &FormatError{Section: typ, Msg: "second SIGN in one analysis block"}
		}
		s, err := decodeSignature(&c, false)
		if err != nil {
			return err
		}
		last.Signature = s
	case "CLCK", "VIDO", "ANNO":
		if err := p.order(stageAncillary, typ); err != nil {
			return err
		}
		switch typ {
		case "CLCK":
			if f.Clock != nil {
				return &FormatError{Section: typ, Msg: "second CLCK"}
			}
			if f.Video != nil {
				return &FormatError{Section: typ, Msg: "CLCK after VIDO"}
			}
			f.Clock = payload
		case "VIDO":
			if f.Video != nil {
				return &FormatError{Section: typ, Msg: "second VIDO"}
			}
			f.Video = payload
		case "ANNO":
			if f.Annotations != nil {
				return &FormatError{Section: typ, Msg: "second ANNO"}
			}
			anns, err := decodeAnnotations(&c)
			if err != nil {
				return err
			}
			f.Annotations = anns
		}
	case "MSIG":
		if err := p.order(stageMsig, typ); err != nil {
			return err
		}
		if f.Match == nil || len(f.Games) == 0 {
			return &FormatError{Section: typ, Msg: "MSIG without MTCH and GAME"}
		}
		if len(f.MatchSignatures) >= MaxMatchSignatures {
			return &FormatError{Section: typ, Msg: "too many MSIG"}
		}
		s, err := decodeSignature(&c, true)
		if err != nil {
			return err
		}
		f.MatchSignatures = append(f.MatchSignatures, *s)
	case "CSUM":
		if err := p.order(stageCsum, typ); err != nil {
			return err
		}
		if headerOff+9+len(payload) != len(p.data)-4 {
			return &FormatError{Section: typ, Msg: "CSUM is not the last section"}
		}
		cs, err := decodeChecksum(&c, p.data[:headerOff])
		if err != nil {
			return err
		}
		f.Checksum = cs
	}
	return nil
}

// check applies the whole-file rules that need every section (2.4, 2.5, 11).
func (p *parser) check() error {
	f := p.f
	if !p.sawCritical && f.Match == nil && len(f.Games) == 0 && len(f.Analyses) == 0 {
		return &FormatError{Msg: "no MTCH, GAME, ANAL or DECS section"}
	}
	shape := f.Shape()
	for i := range f.Analyses {
		a := &f.Analyses[i]
		if a.critical != (shape == ShapeAnalysis) {
			return &FormatError{Section: "ANAL", Msg: "CRITICAL bit does not match the file shape"}
		}
		if shape == ShapeAnalysis && a.MatchDigest == nil {
			return &FormatError{Section: "ANAL", Msg: "Analysis-shape block without match_digest"}
		}
	}
	if shape == ShapeAnalysis && (f.Clock != nil || f.Video != nil || len(f.MatchSignatures) > 0) {
		return &FormatError{Msg: "CLCK, VIDO or MSIG in an Analysis-shape file"}
	}
	if shape == ShapeAnalysis && len(f.Analyses) > 0 && !p.sawCritical {
		return &FormatError{Msg: "Analysis-shape file without a CRITICAL section"}
	}
	if f.Match != nil {
		md := f.MatchDigest()
		for _, a := range f.Analyses {
			if a.MatchDigest != nil && *a.MatchDigest != md {
				return &FormatError{Section: "ANAL", Msg: "match_digest does not match MTCH and GAME"}
			}
		}
		if err := checkScore(f); err != nil {
			return err
		}
	}
	plyCount := f.PlyCount()
	for _, a := range f.Analyses {
		for _, d := range a.Decisions {
			if shape != ShapeAnalysis && d.PlyRef >= plyCount {
				return &FormatError{Section: "DECS", Msg: "ply_ref past the last ply"}
			}
		}
	}
	return nil
}

// MatchDigest computes match_digest (6.1) over the stored MTCH and GAME
// payloads, each preceded by its length.
func (f *File) MatchDigest() [32]byte {
	h := sha256.New()
	var n [4]byte
	write := func(b []byte) {
		binary.LittleEndian.PutUint32(n[:], uint32(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	write(f.mtchPayload)
	for _, g := range f.gamePayloads {
		write(g)
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func decodeChecksum(c *cursor, covered []byte) (*Checksum, error) {
	r, err := c.record()
	if err != nil {
		return nil, err
	}
	alg, err := r.c.v32()
	if err != nil {
		return nil, err
	}
	digest, err := r.c.blob()
	if err != nil {
		return nil, err
	}
	if err := r.finish(0); err != nil {
		return nil, err
	}
	if c.remaining() != 0 {
		return nil, c.errorf("trailing bytes after CSUM record")
	}
	cs := &Checksum{Algorithm: int(alg), Digest: digest}
	switch alg {
	case 0:
		if len(digest) != 4 {
			return nil, c.errorf("CRC32 digest of %d bytes", len(digest))
		}
		if binary.LittleEndian.Uint32(digest) != crc32.ChecksumIEEE(covered) {
			return nil, c.errorf("CRC32 mismatch")
		}
		cs.Verified = true
	case 3:
		if len(digest) != 32 {
			return nil, c.errorf("SHA-256 digest of %d bytes", len(digest))
		}
		sum := sha256.Sum256(covered)
		if !bytes.Equal(sum[:], digest) {
			return nil, c.errorf("SHA-256 mismatch")
		}
		cs.Verified = true
	}
	return cs, nil
}

func instant(c *cursor) (*time.Time, error) {
	v, err := c.varint(64)
	if err != nil {
		return nil, err
	}
	t := time.UnixMilli(int64(v)).UTC()
	return &t, nil
}

func intp(c *cursor) (*int, error) {
	v, err := c.v32()
	if err != nil {
		return nil, err
	}
	n := int(v)
	return &n, nil
}

func equity(c *cursor) (*float64, error) {
	v, err := c.int32le()
	if err != nil {
		return nil, err
	}
	e := float64(v) / EquityScale
	return &e, nil
}

func equityLoss(c *cursor) (*float64, error) {
	v, err := c.u32()
	if err != nil {
		return nil, err
	}
	e := float64(v) / EquityScale
	return &e, nil
}

func probability(c *cursor) (float64, error) {
	v, err := c.u16()
	if err != nil {
		return 0, err
	}
	if v > ProbabilityScale {
		return 0, c.errorf("probability %d above 10000", v)
	}
	return float64(v) / ProbabilityScale, nil
}

func probabilities(c *cursor) (*Probabilities, error) {
	var v [5]float64
	for i := range v {
		x, err := probability(c)
		if err != nil {
			return nil, err
		}
		v[i] = x
	}
	return &Probabilities{v[0], v[1], v[2], v[3], v[4]}, nil
}

func digest(c *cursor) (*[32]byte, error) {
	b, err := c.bytes(32)
	if err != nil {
		return nil, err
	}
	var d [32]byte
	copy(d[:], b)
	return &d, nil
}

func identifier(c *cursor) ([16]byte, error) {
	var id [16]byte
	b, err := c.bytes(16)
	if err != nil {
		return id, err
	}
	copy(id[:], b)
	return id, nil
}

func board(c *cursor) (*Board, error) {
	b, err := c.bytes(26)
	if err != nil {
		return nil, err
	}
	var out Board
	for i := range out {
		out[i] = int8(b[i])
	}
	return &out, nil
}

func step(b byte) (Step, error) {
	s := Step{From: int(b & 0x1f), Pips: int(b >> 5)}
	if s.From > 25 || s.Pips == 7 {
		return s, errors.New("invalid step byte")
	}
	return s, nil
}

func countedSteps(c *cursor) ([]Step, error) {
	n, err := c.v32()
	if err != nil {
		return nil, err
	}
	if n > 64 {
		return nil, c.errorf("%d steps", n)
	}
	out := make([]Step, 0, n)
	for i := 0; i < int(n); i++ {
		b, err := c.u8()
		if err != nil {
			return nil, err
		}
		s, err := step(b)
		if err != nil || b == 0 {
			return nil, c.errorf("invalid step byte %#x", b)
		}
		out = append(out, s)
	}
	return out, nil
}
