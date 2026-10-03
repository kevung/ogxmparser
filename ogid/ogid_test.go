package ogid

import (
	"errors"
	"testing"
)

// The specification's own example (docs/OGID.md): unsorted runs read as the
// opening position, and format back sorted, as the reference writes them.
func TestSpecExample(t *testing.T) {
	o, err := Parse("11jjjjjhhhccccc:ooddddd88866666:N0N::W:IW:0:0:1:0")
	if err != nil {
		t.Fatal(err)
	}
	want := [26]int8{1: 2, 12: 5, 17: 3, 19: 5, 24: -2, 13: -5, 8: -3, 6: -5}
	if o.Board != want {
		t.Errorf("board %v", o.Board)
	}
	if o.CubeValue() != 1 || o.CubeOwner != 'N' || o.Dice != [2]int{} || o.MatchLength != 1 {
		t.Errorf("fields %+v", o)
	}
	if got := o.String(); got != "11ccccchhhjjjjj:66666888dddddoo:N0N::W:IW:0:0:1:0" {
		t.Errorf("String %q", got)
	}
}

// The color field names who reached the position: the other side moves.
func TestToMove(t *testing.T) {
	for s, want := range map[string]int{
		"17ccccghhhjjjjj:66666888dddddoo:N0N:46:W:R:0:0:3:1":  Black,
		"11ccccchhhjjjjj:66666888dddddoo:N0N:46:B:IB:0:0:3:0": White,
	} {
		o, err := Parse(s)
		if err != nil || o.ToMove() != want {
			t.Errorf("%s: to move %d, %v", s, o.ToMove(), err)
		}
	}
	o := New()
	o.SetToMove(Black)
	if o.Reached != 'W' {
		t.Errorf("SetToMove(Black) reached %c", o.Reached)
	}
}

func TestMatchLengthModifiers(t *testing.T) {
	for _, tc := range []struct {
		field          string
		length         int
		crawford, last bool
		maxGames       int
	}{
		{"7", 7, false, false, -1},
		{"7C", 7, true, false, -1},
		{"0L", 0, false, true, -1},
		{"0G5", 0, false, false, 5},
		{"", 0, false, false, -1},
	} {
		o, err := Parse("::N0N::W:C:0:0:" + tc.field + ":0")
		if err != nil {
			t.Fatalf("%q: %v", tc.field, err)
		}
		if o.MatchLength != tc.length || o.Crawford != tc.crawford || o.LastGame != tc.last || o.MaxGames != tc.maxGames {
			t.Errorf("%q: %+v", tc.field, o)
		}
		if tc.field != "" {
			if s := o.String(); s != "::N0N::W:C:0:0:"+tc.field+":0" {
				t.Errorf("%q formats as %q", tc.field, s)
			}
		}
	}
}

func TestCheckerCountField(t *testing.T) {
	s := "123:mno:N0N::B:C:0:0:0:0:3"
	o, err := Parse(s)
	if err != nil || o.Checkers != 3 || o.String() != s {
		t.Errorf("%q: %+v %v", s, o, err)
	}
	if _, err := Parse("1234:mno:N0N::B:C:0:0:0:0:3"); !errors.Is(err, ErrInvalid) {
		t.Errorf("four checkers of three accepted: %v", err)
	}
}

func TestRefusals(t *testing.T) {
	for _, s := range []string{
		"",
		"abc",
		"11z:66:N0N::W",              // character outside 0-p
		"1111111111111111:66:N0N::W", // sixteen checkers
		"1:1:N0N::W",                 // both colours on one point
		"1:6:X0N::W",                 // cube owner
		"1:6:N0Q::W",                 // cube action
		"1:6:N0N:71:W",               // die 7
		"1:6:N0N:1:W",                // one die
		"1:6:N0N::W:C:0:0:7X:0",      // match modifier
		"1:6:N0N::W:C:x:0:7:0",       // score
	} {
		if _, err := Parse(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q accepted: %v", s, err)
		}
	}
}

func TestLooks(t *testing.T) {
	for s, want := range map[string]bool{
		"cccccggggg:ddddiiiiii:N0N:63:W:IW:4:3:7:1:15":        true,
		"OGID=11jjjjjhhhccccc:ooddddd88866666:N0N::W":         true,
		"XGID=-b----E-C---eE---c-e----B-:0:0:1:52:0:0:3:0:10": false,
		"4HPwATDgc/ABMA:cAkAAAAAAAAA":                         false,
		"11:66:N0N::":                                         false,
	} {
		if Looks(s) != want {
			t.Errorf("Looks(%q) = %v", s, !want)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add("cccccggggg:ddddiiiiii:N0N:63:W:IW:4:3:7:1:15")
	f.Add("123:mno:N0N::B:C:0:0:0G5:0:3")
	f.Fuzz(func(t *testing.T, s string) {
		o, err := Parse(s)
		if err != nil {
			return
		}
		again, err := Parse(o.String())
		if err != nil || again != o {
			t.Errorf("%q -> %q does not read back: %v", s, o.String(), err)
		}
	})
}
