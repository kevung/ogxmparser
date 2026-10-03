package ogxmparser

import (
	"fmt"
	"time"
)

// Limits of spec section 10 that this reader enforces.
const (
	MaxFileSize        = 64 << 20
	MaxSectionSize     = 16 << 20
	MaxSections        = 4096
	MaxGames           = 1000
	MaxPliesPerGame    = 1500
	MaxTotalPlies      = 100000
	MaxAltsPerDecision = 1024
	MaxAnalyses        = 64
	MaxString          = 4096
	MaxAnnotations     = 4096
	MaxDrawings        = 256
	MaxDecisions       = 300000
	MaxCoverage        = 100000
	MaxMatchSignatures = 16
)

// EquityScale is the fixed-point unit of the Equity and Equity loss kinds (3.3).
const EquityScale = 1e6

// ProbabilityScale is the fixed-point unit of the Probability kind (3.3).
const ProbabilityScale = 1e4

// FormatError is a hard error (section 0): the file is rejected.
type FormatError struct {
	Section string // four-character section type, empty for the container
	Offset  int    // byte offset within the section payload (or the file)
	Msg     string
}

func (e *FormatError) Error() string {
	if e.Section == "" {
		return fmt.Sprintf("ogxm: offset %d: %s", e.Offset, e.Msg)
	}
	return fmt.Sprintf("ogxm: %s at payload offset %d: %s", e.Section, e.Offset, e.Msg)
}

// Seat is the Seat enum (9.5).
type Seat int

const (
	White   Seat = 0
	Black   Seat = 1
	Centred Seat = 2
)

func (s Seat) String() string {
	switch s {
	case White:
		return "white"
	case Black:
		return "black"
	case Centred:
		return "centred"
	}
	return fmt.Sprintf("seat(%d)", int(s))
}

// Opponent returns the other seat of White or Black.
func (s Seat) Opponent() Seat { return 1 - s }

// Variant is the Variant enum (9.1).
type Variant int

const (
	Backgammon  Variant = 0
	Nackgammon  Variant = 1
	Hypergammon Variant = 2
	Longgammon  Variant = 3
)

// Checkers returns the variant's checkers per side (M1).
func (v Variant) Checkers() int {
	if v == Hypergammon {
		return 3
	}
	return 15
}

// Rules is the rules bitfield (9.2).
type Rules uint64

const (
	RuleCrawford Rules = 1 << iota
	RuleJacoby
	RuleBeavers
	RuleRaccoons
	RuleAutoDoubles
)

// Board is 26 signed counts indexed by absolute point (M1): positive is
// White, negative Black; 0 is White's bar and 25 Black's.
type Board [26]int8

// Player is the nested player record (4.1).
type Player struct {
	UserID       string
	Rating       *float64 // hundredths decoded: 1523.47
	RatingSystem string
	Country      string
	Kind         *int // 9.28
}

// Match is the MTCH record (4).
type Match struct {
	Length              int // 0 = money session
	Variant             Variant
	WhiteName           string
	BlackName           string
	Rules               Rules
	CubeLimit           int // 0 = no limit
	ScoreStart          [2]int
	ScoreFinal          *[2]int // White, Black
	Result              int     // 9.3; 0 = absent (unfinished, or decided by the score)
	Source              *int    // 9.4
	StartedAt           *time.Time
	CompletedAt         *time.Time
	PlayerSeat          *Seat
	CrawfordBeforeStart bool
	Event               string
	EventYear           int
	DatePrecision       *int // 9.27
	Stage               string
	Round               int
	Table               string
	City                string
	Country             string
	EventURL            string
	Site                string
	MatchRef            string
	WhiteProfile        *Player
	BlackProfile        *Player
	Rated               bool
}

// Game is one GAME section (5).
type Game struct {
	Winner           *Seat
	PointsWon        *int
	IsLastGame       bool
	InitialBoard     *Board // nil = the variant's opening position (5.2)
	InitialCubeValue int    // 1 when absent
	InitialCubeOwner Seat   // Centred when absent
	AutoDoubles      int
	Termination      *int // 9.7
	Plies            []Ply
}

// Action ids of 9.14 beyond the dice rolls 0-20.
const (
	ActionDouble      = 21
	ActionTake        = 22
	ActionDrop        = 23
	ActionGameOver    = 24
	ActionMatchOver   = 25
	ActionFinal       = 26
	ActionResignGame  = 27
	ActionResignMatch = 28
	ActionForfeit     = 29
	ActionNull        = 30
	ActionSetPosition = 31
	ActionBeaver      = 32
	ActionRaccoon     = 33
	ActionSettle      = 34
	ActionReserved    = 35
	ActionCubeSet     = 36
	ActionPass        = 37
	actionEscape      = 63
)

// diceOfAction maps a dice action id (0-20) to its dice, smaller die first.
var diceOfAction = func() (t [21][2]int) {
	id := 0
	for a := 1; a <= 6; a++ {
		for b := a; b <= 6; b++ {
			t[id] = [2]int{a, b}
			id++
		}
	}
	return t
}()

// DiceAction returns the action id of a roll (9.14).
func DiceAction(d1, d2 int) int {
	if d1 > d2 {
		d1, d2 = d2, d1
	}
	offset := [7]int{0, 0, 6, 11, 15, 18, 20}
	return offset[d1] + d2 - d1
}

// Step moves one checker (5.1).
type Step struct {
	From int // Point, 0-25
	Pips int // 1-6
}

// Ply is one ply record (5.1), with its extras decoded.
type Ply struct {
	Ref    int // ply_ref (I2)
	Action int // the real action id, escape resolved
	Seat   Seat
	Steps  []Step

	ExtraDice   *[2]int
	ResignValue *int
	CubeValue   *int
	Illegal     bool
	SettleValue *float64
	Board       *Board
	CubeOwner   *Seat
}

// IsDiceAction reports whether the ply is a dice roll (actions 0-20).
func (p Ply) IsDiceAction() bool { return p.Action >= 0 && p.Action <= 20 }

// Dice returns the roll of a dice action, smaller die first.
func (p Ply) Dice() [2]int {
	if !p.IsDiceAction() {
		return [2]int{}
	}
	return diceOfAction[p.Action]
}

// Currency is the Currency enum (9.10).
type Currency int

const (
	Cubeless      Currency = 0
	CubefulMoney  Currency = 1
	CubefulMatch  Currency = 2
	CurrencyUnset Currency = -1
)

// Rollout is the nested rollout record (6.2).
type Rollout struct {
	Trials            *int
	TruncationDepth   *int
	MovePly           *int
	VarianceReduction *int
	Seed              *uint64
	BudgetMS          *int
	MatchPolicy       *int
}

// Level is the nested level record (6.4), at one tier.
type Level struct {
	Preset     *string
	CheckerPly *int
	CubePly    *int
	Rollout    *Rollout
	NoRollout  bool
}

// Probabilities are cumulative, from the decision owner's view (3.3).
type Probabilities struct {
	Win, GammonWin, BackgammonWin, GammonLoss, BackgammonLoss float64
}

// Alternative is one candidate play of a checker decision (7.2).
type Alternative struct {
	Steps          []Step
	Equity         float64
	Probs          *Probabilities
	Level          *Level
	IsPlayed       bool
	RolloutSE      *float64
	CubelessEquity *float64
}

// CheckerDecision is a CHECKER record (7.1).
type CheckerDecision struct {
	Alternatives      []Alternative
	AlternativesTotal *int
	BestEquity        *float64 // stored only without alternatives
	EquityLoss        *float64 // stored only when no alternative is played
	Level             *Level
	PositionTags      *uint64
	ProducerRef       *int
}

// Verdict is the cube verdict enum (9.13).
type Verdict int

const (
	VerdictNoDouble Verdict = 0
	VerdictDouble   Verdict = 1
	VerdictTake     Verdict = 2
	VerdictPass     Verdict = 3
	VerdictTooGood  Verdict = 4
	VerdictBeaver   Verdict = 5
	VerdictRaccoon  Verdict = 6
)

// CubeDecision is a CUBE record (7.4).
type CubeDecision struct {
	Verdict          Verdict
	NoDoubleEquity   *float64
	DoubleTakeEquity *float64
	DoublePassEquity *float64
	Probs            *Probabilities
	EquityLoss       *float64
	TakePoint        *float64
	WindowSearched   bool
	Level            *Level
	IsOptional       bool
	IsFreeCube       bool
	CubefulTakeValue *float64
	Currency         *Currency // nil = the block's
	ProducerRef      *int
}

// ResignDecision is a RESIGN record (7.5).
type ResignDecision struct {
	CorrectValue    *int
	ResignError     *float64
	TakeResignError *float64
	Probs           *Probabilities
	EquityLoss      *float64
	Level           *Level
	ProducerRef     *int
}

// RollDecision is a ROLL record (7.6).
type RollDecision struct {
	Luck        float64
	Level       *Level
	ProducerRef *int
}

// DecisionKind is the decision kind enum (9.12).
type DecisionKind int

const (
	KindChecker DecisionKind = 0
	KindCube    DecisionKind = 1
	KindResign  DecisionKind = 2
	KindRoll    DecisionKind = 3
)

// Decision is one DECS record (7). Exactly one of the kind pointers is set
// for a known kind; an unknown kind leaves all nil.
type Decision struct {
	PlyRef  int
	Kind    DecisionKind
	Checker *CheckerDecision
	Cube    *CubeDecision
	Resign  *ResignDecision
	Roll    *RollDecision
}

// Analysis is one analysis block: ANAL, its DECS and its optional SIGN (6-8.1).
type Analysis struct {
	ID             [16]byte
	MatchDigest    *[32]byte
	Producer       *int // 9.9
	Level          *Level
	Complete       bool
	Coverage       []int // nil = every ply
	ModelID        string
	ModelName      string
	ModelDigest    *[32]byte
	EngineBuild    string
	Currency       Currency
	CubeEfficiency *float64
	METID          string
	Tables         string
	StartedAt      *time.Time
	CompletedAt    *time.Time
	DurationMS     *int
	Sources        [][16]byte
	Decisions      []Decision
	Signature      *Signature

	analPayload []byte
	decsPayload []byte
	critical    bool
}

// Signature is a SIGN (8.1) or MSIG (8.6) record.
type Signature struct {
	Algorithm int // 9.16: 1 = Ed25519
	Signature []byte
	KeyID     string
	PublicKey []byte
	SignedAt  *time.Time
	Covers    uint64 // MSIG only, 9.26
}

// Annotation is one ANNO record (8.4). Drawings are counted, not decoded.
type Annotation struct {
	Scope    int // 9.20
	Ref      int
	Value    string
	Key      *string
	Kind     *DecisionKind
	AltIndex *int
	Lang     string
	Author   string
	At       *time.Time
	Drawings int
	Analysis *[16]byte
}

// Checksum is the CSUM record (8.5).
type Checksum struct {
	Algorithm int // 9.15: 0 CRC32, 3 SHA-256
	Digest    []byte
	Verified  bool // false only when the algorithm is unknown to this reader
}

// Shape is the file shape of 2.5.
type Shape int

const (
	ShapeMatch Shape = iota
	ShapeAnalysis
	ShapeCombined
)

// File is a parsed OGXM v2 file.
type File struct {
	VersionMajor, VersionMinor     uint16
	MinReaderMajor, MinReaderMinor uint16

	Match           *Match
	Games           []Game
	Analyses        []Analysis
	Annotations     []Annotation
	MatchSignatures []Signature
	Checksum        *Checksum
	// Clock and Video are the raw CLCK and VIDO payloads; this reader does not
	// interpret them (they are ancillary), but an MSIG may cover the clock.
	Clock []byte
	Video []byte

	mtchPayload  []byte
	gamePayloads [][]byte
}

// Shape returns the file's shape (2.5).
func (f *File) Shape() Shape {
	switch {
	case f.Match == nil:
		return ShapeAnalysis
	case len(f.Analyses) > 0:
		return ShapeCombined
	}
	return ShapeMatch
}

// PlyCount returns the number of plies across every game.
func (f *File) PlyCount() int {
	n := 0
	for _, g := range f.Games {
		n += len(g.Plies)
	}
	return n
}
