package ogxmparser

import (
	"fmt"
	"math"

	"github.com/kevung/ogxmparser/ogid"
)

// openings are the boards of 5.2.
var openings = map[Variant]Board{
	Backgammon:  {1: 2, 12: 5, 17: 3, 19: 5, 24: -2, 13: -5, 8: -3, 6: -5},
	Nackgammon:  {1: 2, 2: 2, 12: 4, 17: 3, 19: 4, 24: -2, 23: -2, 13: -4, 8: -3, 6: -4},
	Hypergammon: {1: 1, 2: 1, 3: 1, 24: -1, 23: -1, 22: -1},
	Longgammon:  {1: 15, 24: -15},
}

// Opening returns the variant's opening position (5.2).
func Opening(v Variant) (Board, bool) {
	b, ok := openings[v]
	return b, ok
}

// Count returns the checkers of seat on the board.
func (b *Board) Count(seat Seat) int {
	n := 0
	for _, v := range b {
		if seat == White && v > 0 {
			n += int(v)
		} else if seat == Black && v < 0 {
			n -= int(v)
		}
	}
	return n
}

// Apply plays the steps of seat in stored order (M3). It checks that each
// step moves a checker the seat holds, enters from the bar first, lands on a
// point the opponent does not hold, and bears off only with every checker
// home. It does not check M4 (that the play is a legal one for the dice).
func (b Board) Apply(seat Seat, steps []Step) (Board, error) {
	for i, s := range steps {
		if s.Pips < 1 || s.Pips > 6 {
			return b, fmt.Errorf("step %d: %d pips", i, s.Pips)
		}
		own := func(v int8) bool { return (seat == White && v > 0) || (seat == Black && v < 0) }
		if !own(b[s.From]) {
			return b, fmt.Errorf("step %d: no checker of the seat on point %d", i, s.From)
		}
		bar := 0
		if seat == Black {
			bar = 25
		}
		if s.From != bar && own(b[bar]) {
			return b, fmt.Errorf("step %d: a checker is on the bar", i)
		}
		dir := 1
		if seat == Black {
			dir = -1
		}
		to := s.From + dir*s.Pips
		if seat == White {
			b[s.From]--
		} else {
			b[s.From]++
		}
		if (seat == White && to >= 25) || (seat == Black && to <= 0) {
			lo, hi := 0, 18
			if seat == Black {
				lo, hi = 7, 25
			}
			for p := lo; p <= hi; p++ {
				if own(b[p]) {
					return b, fmt.Errorf("step %d: bears off with a checker outside home", i)
				}
			}
			continue
		}
		switch {
		case seat == White && b[to] == -1:
			b[to] = 0
			b[25]--
		case seat == Black && b[to] == 1:
			b[to] = 0
			b[0]++
		case seat == White && b[to] < -1, seat == Black && b[to] > 1:
			return b, fmt.Errorf("step %d: point %d is blocked", i, to)
		}
		if seat == White {
			b[to]++
		} else {
			b[to]--
		}
	}
	return b, nil
}

// PlyState is one ply's reconstructed position, before and after it, as the
// OGIDs the reference implementation derives (ogid_before, ogid_after).
type PlyState struct {
	Before, After ogid.OGID
	// UnplayedRoll marks a dice action with no step followed by a
	// resignation or forfeit (5.4.5): not a checker decision.
	UnplayedRoll bool
}

// GameReplay is one game's replay.
type GameReplay struct {
	Plies []PlyState // one per ply up to FailedAt
	// FailedAt is the index of the first ply that does not replay, or -1
	// (M9). Plies from it on have no position.
	FailedAt int
	Err      error
	Crawford bool
	// ScoreBefore is the score at the game's start, White then Black (M8).
	ScoreBefore [2]int
}

// Replay reconstructs every game's positions (5.4).
func (f *File) Replay() []GameReplay {
	if f.Match == nil {
		return nil
	}
	walk := scoreWalk(f)
	out := make([]GameReplay, len(f.Games))
	for gi := range f.Games {
		out[gi] = replayGame(f, gi, walk)
	}
	return out
}

type scores struct {
	before   [][2]float64
	crawford []bool
	final    [2]float64
}

// scoreWalk applies M8 and derives the Crawford game (5).
func scoreWalk(f *File) scores {
	m := f.Match
	s := scores{before: make([][2]float64, len(f.Games)), crawford: make([]bool, len(f.Games))}
	cur := [2]float64{float64(m.ScoreStart[0]), float64(m.ScoreStart[1])}
	crawfordDone := m.CrawfordBeforeStart
	for gi, g := range f.Games {
		s.before[gi] = cur
		if m.Length > 0 && m.Rules&RuleCrawford != 0 && !crawfordDone {
			L := float64(m.Length - 1)
			if cur[0] == L || cur[1] == L {
				s.crawford[gi] = true
				crawfordDone = true
			}
		}
		if g.Winner == nil || *g.Winner > Black {
			continue
		}
		w := *g.Winner
		if g.PointsWon != nil {
			pts := float64(*g.PointsWon)
			if m.Length > 0 {
				pts = math.Min(pts, float64(m.Length)-cur[w])
			}
			cur[w] += pts
		} else if m.Length == 0 {
			for i := len(g.Plies) - 1; i >= 0; i-- {
				if p := g.Plies[i]; p.Action == ActionSettle && p.SettleValue != nil {
					cur[w] += math.Abs(*p.SettleValue)
					break
				}
			}
		}
	}
	s.final = cur
	return s
}

// checkScore verifies score_final against M8 (section 11).
func checkScore(f *File) error {
	if f.Match.ScoreFinal == nil {
		return nil
	}
	got := scoreWalk(f).final
	want := *f.Match.ScoreFinal
	if got[0] != float64(want[0]) || got[1] != float64(want[1]) {
		return &FormatError{Section: "MTCH", Msg: fmt.Sprintf("score_final %v does not match the games (%v)", want, got)}
	}
	return nil
}

func log2(v int) int {
	n := 0
	for v > 1 {
		v >>= 1
		n++
	}
	return n
}

func ownerChar(s Seat) byte {
	switch s {
	case White:
		return 'W'
	case Black:
		return 'B'
	}
	return 'N'
}

func replayGame(f *File, gi int, walk scores) GameReplay {
	m, g := f.Match, &f.Games[gi]
	rp := GameReplay{FailedAt: -1, Crawford: walk.crawford[gi]}
	rp.ScoreBefore = [2]int{int(walk.before[gi][0]), int(walk.before[gi][1])}

	fail := func(i int, err error) GameReplay {
		rp.FailedAt, rp.Err = i, err
		return rp
	}
	board, ok := openings[m.Variant]
	if g.InitialBoard != nil {
		board = *g.InitialBoard
	} else if !ok {
		return fail(0, fmt.Errorf("unknown variant %d", m.Variant))
	}
	n := m.Variant.Checkers()
	if board.Count(White) > n || board.Count(Black) > n {
		return fail(0, fmt.Errorf("initial board holds more than %d checkers", n))
	}

	base := ogid.New()
	base.Score = rp.ScoreBefore
	base.MatchLength = m.Length
	base.Crawford = rp.Crawford
	base.LastGame = g.IsLastGame
	if n != 15 {
		base.Checkers = n
	}
	cubeLog2 := log2(g.InitialCubeValue) + g.AutoDoubles
	cubeOwner := ownerChar(g.InitialCubeOwner)
	cubeAction := byte('N')
	state := "IB"
	moveID := 0
	pending, afterBeaver, firstRoll := false, false, true
	preDouble := 0

	mk := func(b Board, toMove Seat, dice [2]int, action byte, st string) ogid.OGID {
		o := base
		o.Board = b
		o.SetToMove(int(toMove))
		o.Dice = dice
		o.CubeLog2, o.CubeOwner, o.CubeAction = cubeLog2, cubeOwner, action
		o.GameState = st
		o.MoveID = moveID
		return o
	}
	for i := range g.Plies {
		p := &g.Plies[i]
		a := p.Action
		isResponse := a == ActionTake || a == ActionDrop || a == ActionBeaver
		endsByChoice := a == ActionResignGame || a == ActionResignMatch || a == ActionForfeit
		raccoon := a == ActionRaccoon
		own := ownerChar(p.Seat)
		if (isResponse != pending && !(pending && endsByChoice)) || (raccoon && !afterBeaver) ||
			(a == ActionDouble && cubeOwner != 'N' && cubeOwner != own) {
			return fail(i, fmt.Errorf("ply %d: action %d out of turn", p.Ref, a))
		}
		afterBeaver = false
		var ps PlyState
		if p.IsDiceAction() && len(p.Steps) == 0 && i+1 < len(g.Plies) {
			na := g.Plies[i+1].Action
			ps.UnplayedRoll = na == ActionResignGame || na == ActionResignMatch || na == ActionForfeit
		}
		st := "R"
		if firstRoll {
			st = "IB"
		}
		switch {
		case p.IsDiceAction():
			ps.Before = mk(board, p.Seat, p.Dice(), cubeAction, st)
		case isResponse:
			ps.Before = mk(board, p.Seat, [2]int{}, 'O', "D")
		default:
			ps.Before = mk(board, p.Seat, [2]int{}, cubeAction, state)
		}

		after := board
		switch {
		case p.IsDiceAction():
			nb, err := board.Apply(p.Seat, p.Steps)
			if err != nil {
				return fail(i, fmt.Errorf("ply %d: %w", p.Ref, err))
			}
			after = nb
		case a == ActionSetPosition:
			after = *p.Board
			if after.Count(White) > n || after.Count(Black) > n {
				return fail(i, fmt.Errorf("ply %d: set position holds more than %d checkers", p.Ref, n))
			}
		}

		afterState, afterAction := state, cubeAction
		switch {
		case p.IsDiceAction() || a == ActionPass:
			moveID++
			firstRoll = false
			afterState, afterAction = "C", 'N'
		case a == ActionDouble:
			pending = true
			preDouble = cubeLog2
			afterState, afterAction = "D", 'O'
		case a == ActionTake || a == ActionBeaver || raccoon:
			pending = false
			afterBeaver = a == ActionBeaver
			step := map[int]int{ActionTake: 1, ActionBeaver: 2, ActionRaccoon: 3}[a]
			cubeLog2 = preDouble + step
			cubeOwner = own
			afterState, afterAction = "A", 'T'
		case a == ActionDrop:
			pending = false
			afterState, afterAction = "G", 'P'
		case a == ActionCubeSet:
			cubeLog2 = log2(*p.CubeValue)
			cubeOwner = 'N'
			if p.CubeOwner != nil {
				cubeOwner = ownerChar(*p.CubeOwner)
			}
		}
		next := p.Seat.Opponent()
		if a == ActionSetPosition || a == ActionCubeSet {
			next = p.Seat
		}
		ps.After = mk(after, next, [2]int{}, afterAction, afterState)
		state, cubeAction = afterState, afterAction
		board = after
		rp.Plies = append(rp.Plies, ps)
	}
	return rp
}
