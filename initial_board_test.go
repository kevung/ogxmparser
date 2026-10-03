package ogxmparser

import "testing"

// A default is never written (3.2): an initial_board equal to the variant's
// opening position is refused, any other board is read.
func TestInitialBoardAtOpeningIsRefused(t *testing.T) {
	open := openings[Backgammon]
	record := func(b Board) []byte {
		rec := []byte{27, 1 << 3}
		for _, v := range b {
			rec = append(rec, byte(v))
		}
		return rec
	}
	newParser := func() *parser {
		return &parser{f: &File{Match: &Match{Variant: Backgammon}}}
	}

	if _, err := newParser().decodeGame(&cursor{b: record(open)}); err == nil {
		t.Error("initial_board at the opening position accepted")
	}

	other := open
	other[1], other[2] = 1, 1
	g, err := newParser().decodeGame(&cursor{b: record(other)})
	if err != nil {
		t.Fatalf("other board refused: %v", err)
	}
	if g.InitialBoard == nil || *g.InitialBoard != other {
		t.Errorf("initial board: %+v", g.InitialBoard)
	}
}
