// Package ogid reads and writes OGID, the OpenGammon position ID.
//
// The format is specified in docs/OGID.md of HedgeHog's public repository
// (gitlab.com/eranlambooij/hedgehog-public, CC BY 4.0). Parse follows the
// reference parser, src/ogid.cpp (MIT), field by field, and String writes
// exactly what its ogid_format_full writes; the tests hold both to strings the
// reference produced.
//
//	white:black:cube:dice:color:state:score_w:score_b:match:move_id[:checkers]
//
// The board is absolute (spec M1 of OGXM): point 0 is White's bar, 25
// Black's; White moves up toward 25. Each checker is one character, sorted,
// borne-off checkers omitted.
//
// The color field names the player who REACHED the position, so the player
// to move is the other one: see ToMove.
package ogid

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Seats, as in OGXM (9.5).
const (
	White = 0
	Black = 1
)

// ErrInvalid is wrapped by every parse error.
var ErrInvalid = errors.New("invalid OGID")

// OGID is a decoded OGID string.
type OGID struct {
	// Board holds signed counts by absolute point: positive White,
	// negative Black.
	Board [26]int8

	CubeOwner  byte // 'N' centred, 'W', 'B', 'D' dead, '?' unknown
	CubeLog2   int  // the cube is 1 << CubeLog2
	CubeAction byte // 'N' none, 'O' offered, 'T' taken, 'P' passed

	Dice [2]int // {0, 0} when not rolled

	// Reached is the color field: the player who reached this position,
	// 'W' or 'B'. The reference reads an absent field as 'B' (White to move).
	Reached byte

	GameState string // IW, IB, R, C, A, D, P, PR, G, M, F, RG, RM, FF

	Score       [2]int // points scored, White then Black
	MatchLength int    // 0 = money
	Crawford    bool   // modifier C: this game is the Crawford game
	LastGame    bool   // modifier L: last game of a money session
	MaxGames    int    // modifier G: games left; -1 when absent

	MoveID   int
	Checkers int // checkers per side; 15 when absent
}

// New returns an OGID holding the reference's defaults (ogid_info_init).
func New() OGID {
	return OGID{CubeOwner: 'N', CubeAction: 'N', Reached: 'B', GameState: "IW", MaxGames: -1, Checkers: 15}
}

// ToMove returns the seat to move: the opponent of Reached.
func (o *OGID) ToMove() int {
	if o.Reached == 'W' {
		return Black
	}
	return White
}

// SetToMove sets Reached so that seat is to move.
func (o *OGID) SetToMove(seat int) {
	if seat == White {
		o.Reached = 'B'
	} else {
		o.Reached = 'W'
	}
}

// CubeValue returns the cube's value, 1 << CubeLog2.
func (o *OGID) CubeValue() int { return 1 << o.CubeLog2 }

func pipChar(p int) byte {
	if p < 10 {
		return byte('0' + p)
	}
	return byte('a' + p - 10)
}

func charPip(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'p':
		return int(c-'a') + 10
	}
	return -1
}

// Looks reports whether s has the shape of an OGID: the minimum pattern of
// the specification, ^[0-9a-p]*:[0-9a-p]*:[BWND?]\d+[OTPN]:\d{0,2}:[WB].
func Looks(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) >= 5 && strings.EqualFold(s[:5], "OGID=") {
		s = s[5:]
	}
	f := strings.Split(s, ":")
	if len(f) < 5 {
		return false
	}
	for _, run := range f[:2] {
		for i := 0; i < len(run); i++ {
			if charPip(run[i]) < 0 {
				return false
			}
		}
	}
	c := f[2]
	if len(c) < 3 || !strings.ContainsRune("BWND?", rune(c[0])) || !strings.ContainsRune("OTPN", rune(c[len(c)-1])) {
		return false
	}
	if _, err := strconv.Atoi(c[1 : len(c)-1]); err != nil {
		return false
	}
	if len(f[3]) > 2 {
		return false
	}
	for i := 0; i < len(f[3]); i++ {
		if f[3][i] < '0' || f[3][i] > '9' {
			return false
		}
	}
	return f[4] != "" && (f[4][0] == 'W' || f[4][0] == 'B')
}

// Parse decodes an OGID. An "OGID=" prefix and surrounding blanks are
// accepted. Like the reference, it needs only the two position fields; the
// others default as in New. Unlike the reference, which silently keeps the
// first 15 characters of a run, it refuses a side with more checkers than the
// checker count.
func Parse(s string) (OGID, error) {
	o := New()
	s = strings.TrimSpace(s)
	if len(s) >= 5 && strings.EqualFold(s[:5], "OGID=") {
		s = s[5:]
	}
	f := strings.Split(s, ":")
	if len(f) < 2 {
		return o, fmt.Errorf("%w: fewer than two fields", ErrInvalid)
	}
	if len(f) > 11 {
		return o, fmt.Errorf("%w: more than eleven fields", ErrInvalid)
	}
	field := func(i int) string {
		if i < len(f) {
			return f[i]
		}
		return ""
	}
	var err error
	if v := field(10); v != "" {
		if o.Checkers, err = strconv.Atoi(v); err != nil || o.Checkers < 1 || o.Checkers > 15 {
			return o, fmt.Errorf("%w: checker count %q", ErrInvalid, v)
		}
	}
	for side, run := range f[:2] {
		if len(run) > o.Checkers {
			return o, fmt.Errorf("%w: %d checkers for one side", ErrInvalid, len(run))
		}
		for i := 0; i < len(run); i++ {
			p := charPip(run[i])
			if p < 0 {
				return o, fmt.Errorf("%w: character %q", ErrInvalid, run[i])
			}
			if side == White {
				if o.Board[p] < 0 {
					return o, fmt.Errorf("%w: point %d holds both colours", ErrInvalid, p)
				}
				o.Board[p]++
			} else {
				if o.Board[p] > 0 {
					return o, fmt.Errorf("%w: point %d holds both colours", ErrInvalid, p)
				}
				o.Board[p]--
			}
		}
	}
	if c := field(2); c != "" {
		if err := o.parseCube(c); err != nil {
			return o, err
		}
	}
	if d := field(3); d != "" {
		if len(d) != 2 || d[0] < '1' || d[0] > '6' || d[1] < '1' || d[1] > '6' {
			return o, fmt.Errorf("%w: dice %q", ErrInvalid, d)
		}
		o.Dice = [2]int{int(d[0] - '0'), int(d[1] - '0')}
	}
	if c := field(4); c != "" {
		if c[0] == 'W' {
			o.Reached = 'W'
		} else {
			o.Reached = 'B'
		}
	}
	if st := field(5); st != "" {
		o.GameState = st
	}
	for i := 0; i < 2; i++ {
		if v := field(6 + i); v != "" {
			if o.Score[i], err = strconv.Atoi(v); err != nil || o.Score[i] < 0 {
				return o, fmt.Errorf("%w: score %q", ErrInvalid, v)
			}
		}
	}
	if err := o.parseMatch(field(8)); err != nil {
		return o, err
	}
	if v := field(9); v != "" {
		if o.MoveID, err = strconv.Atoi(v); err != nil {
			return o, fmt.Errorf("%w: move id %q", ErrInvalid, v)
		}
	}
	return o, nil
}

func (o *OGID) parseCube(c string) error {
	if len(c) < 2 || !strings.ContainsRune("WBND?", rune(c[0])) {
		return fmt.Errorf("%w: cube %q", ErrInvalid, c)
	}
	o.CubeOwner = c[0]
	i := 1
	v := 0
	for i < len(c) && c[i] >= '0' && c[i] <= '9' {
		v = v*10 + int(c[i]-'0')
		i++
		if v > 15 {
			return fmt.Errorf("%w: cube %q", ErrInvalid, c)
		}
	}
	o.CubeLog2 = v
	o.CubeAction = 'N'
	if i < len(c) {
		if !strings.ContainsRune("NOTP", rune(c[i])) || i+1 != len(c) {
			return fmt.Errorf("%w: cube %q", ErrInvalid, c)
		}
		o.CubeAction = c[i]
	}
	return nil
}

func (o *OGID) parseMatch(m string) error {
	o.MatchLength, o.Crawford, o.LastGame, o.MaxGames = 0, false, false, -1
	i := 0
	for i < len(m) && m[i] >= '0' && m[i] <= '9' {
		o.MatchLength = o.MatchLength*10 + int(m[i]-'0')
		i++
		if o.MatchLength > 1<<20 {
			return fmt.Errorf("%w: match length %q", ErrInvalid, m)
		}
	}
	if i == len(m) {
		return nil
	}
	switch m[i] {
	case 'C':
		o.Crawford = true
		i++
	case 'L':
		o.LastGame = true
		i++
	case 'G':
		i++
		o.MaxGames = 0
		for i < len(m) && m[i] >= '0' && m[i] <= '9' {
			o.MaxGames = o.MaxGames*10 + int(m[i]-'0')
			i++
			if o.MaxGames > 1<<20 {
				return fmt.Errorf("%w: match length %q", ErrInvalid, m)
			}
		}
	}
	if i != len(m) {
		return fmt.Errorf("%w: match length %q", ErrInvalid, m)
	}
	return nil
}

// Position returns fields 1 and 2: the checkers, sorted, borne-off omitted.
func (o *OGID) Position() string {
	var w, b []int
	for p, n := range o.Board {
		for ; n > 0; n-- {
			w = append(w, p)
		}
		for ; n < 0; n++ {
			b = append(b, p)
		}
	}
	sort.Ints(w)
	sort.Ints(b)
	run := func(ps []int) string {
		out := make([]byte, len(ps))
		for i, p := range ps {
			out[i] = pipChar(p)
		}
		return string(out)
	}
	return run(w) + ":" + run(b)
}

// String formats the full OGID as the reference's ogid_format_full does:
// ten fields, and the eleventh only when the checker count is not 15.
func (o *OGID) String() string {
	var sb strings.Builder
	sb.WriteString(o.Position())
	fmt.Fprintf(&sb, ":%c%d%c:", o.CubeOwner, o.CubeLog2, o.CubeAction)
	if o.Dice[0] != 0 && o.Dice[1] != 0 {
		fmt.Fprintf(&sb, "%d%d", o.Dice[0], o.Dice[1])
	}
	reached := o.Reached
	if reached != 'W' {
		reached = 'B'
	}
	fmt.Fprintf(&sb, ":%c:%s:%d:%d:%d", reached, o.GameState, o.Score[0], o.Score[1], o.MatchLength)
	switch {
	case o.Crawford:
		sb.WriteByte('C')
	case o.LastGame:
		sb.WriteByte('L')
	case o.MaxGames >= 0:
		fmt.Fprintf(&sb, "G%d", o.MaxGames)
	}
	fmt.Fprintf(&sb, ":%d", o.MoveID)
	checkers := o.Checkers
	if checkers == 0 {
		checkers = 15
	}
	if checkers != 15 {
		fmt.Fprintf(&sb, ":%d", checkers)
	}
	return sb.String()
}
