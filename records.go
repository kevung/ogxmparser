package ogxmparser

import (
	"strings"
)

func decodePlayer(c *cursor) (*Player, error) {
	r, err := c.record()
	if err != nil {
		return nil, err
	}
	if r.mask == 0 {
		return nil, c.errorf("empty player record")
	}
	p := &Player{}
	if r.has(0) {
		if p.UserID, err = r.c.str(); err != nil {
			return nil, err
		}
	}
	if r.has(1) {
		v, err := r.c.v32()
		if err != nil {
			return nil, err
		}
		x := float64(v) / 100
		p.Rating = &x
	}
	if r.has(2) {
		if p.RatingSystem, err = r.c.str(); err != nil {
			return nil, err
		}
	}
	if r.has(1) != r.has(2) {
		return nil, c.errorf("rating without rating_system, or the reverse")
	}
	if r.has(3) {
		if p.Country, err = r.c.str(); err != nil {
			return nil, err
		}
		if !isCountry(p.Country) {
			return nil, c.errorf("country %q is not ISO 3166-1 alpha-2", p.Country)
		}
	}
	if r.has(4) {
		if p.Kind, err = intp(&r.c); err != nil {
			return nil, err
		}
	}
	return p, r.finish(5)
}

func isCountry(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}

func decodeMatch(c *cursor) (*Match, error) {
	r, err := c.record()
	if err != nil {
		return nil, err
	}
	if c.remaining() != 0 {
		return nil, c.errorf("trailing bytes after MTCH record")
	}
	m := &Match{}
	length, err := r.c.v32()
	if err != nil {
		return nil, err
	}
	variant, err := r.c.v32()
	if err != nil {
		return nil, err
	}
	m.Length, m.Variant = int(length), Variant(variant)
	rc := &r.c
	str := func(bit uint, dst *string, maxLen int) error {
		if !r.has(bit) {
			return nil
		}
		s, err := rc.str()
		if err != nil {
			return err
		}
		if bit >= 12 && (s == "" || (maxLen > 0 && len(s) > maxLen)) {
			return rc.errorf("match context string of bit %d empty or too long", bit)
		}
		*dst = s
		return nil
	}
	if err := str(0, &m.WhiteName, 0); err != nil {
		return nil, err
	}
	if err := str(1, &m.BlackName, 0); err != nil {
		return nil, err
	}
	if r.has(2) {
		v, err := rc.varint(64)
		if err != nil {
			return nil, err
		}
		m.Rules = Rules(v)
	}
	if r.has(3) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		if !isPow2(v) {
			return nil, rc.errorf("cube_limit %d is not a power of two", v)
		}
		m.CubeLimit = int(v)
	}
	if r.has(4) {
		for i := 0; i < 2; i++ {
			v, err := rc.v32()
			if err != nil {
				return nil, err
			}
			m.ScoreStart[i] = int(v)
		}
		if m.ScoreStart == [2]int{} {
			return nil, rc.errorf("score_start written at its default")
		}
	}
	if r.has(5) {
		var s [2]int
		for i := 0; i < 2; i++ {
			v, err := rc.v32()
			if err != nil {
				return nil, err
			}
			s[i] = int(v)
		}
		m.ScoreFinal = &s
	}
	if r.has(6) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		if v == 0 {
			return nil, rc.errorf("result 0")
		}
		m.Result = int(v)
	}
	if r.has(7) {
		if m.Source, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if r.has(8) {
		if m.StartedAt, err = instant(rc); err != nil {
			return nil, err
		}
	}
	if r.has(9) {
		if m.CompletedAt, err = instant(rc); err != nil {
			return nil, err
		}
	}
	if r.has(10) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		s := Seat(v)
		m.PlayerSeat = &s
	}
	m.CrawfordBeforeStart = r.has(11)
	if err := str(12, &m.Event, 120); err != nil {
		return nil, err
	}
	if r.has(13) {
		if !r.has(12) {
			return nil, rc.errorf("event_year without event")
		}
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		m.EventYear = int(v)
	}
	if r.has(14) {
		if !r.has(8) {
			return nil, rc.errorf("date_precision without started_at")
		}
		if m.DatePrecision, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if err := str(15, &m.Stage, 60); err != nil {
		return nil, err
	}
	if r.has(16) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		if v < 1 || v > 99 {
			return nil, rc.errorf("round %d outside 1-99", v)
		}
		m.Round = int(v)
	}
	if err := str(17, &m.Table, 24); err != nil {
		return nil, err
	}
	if err := str(18, &m.City, 80); err != nil {
		return nil, err
	}
	if err := str(19, &m.Country, 0); err != nil {
		return nil, err
	}
	if r.has(19) && !isCountry(m.Country) {
		return nil, rc.errorf("country %q is not ISO 3166-1 alpha-2", m.Country)
	}
	if err := str(20, &m.EventURL, 512); err != nil {
		return nil, err
	}
	if r.has(20) && !strings.HasPrefix(m.EventURL, "https://") {
		return nil, rc.errorf("event_url does not begin https://")
	}
	if err := str(21, &m.Site, 253); err != nil {
		return nil, err
	}
	if err := str(22, &m.MatchRef, 64); err != nil {
		return nil, err
	}
	if r.has(22) && (!r.has(21) || m.MatchRef == "." || m.MatchRef == "..") {
		return nil, rc.errorf("invalid match_ref")
	}
	if r.has(23) {
		if m.WhiteProfile, err = decodePlayer(rc); err != nil {
			return nil, err
		}
	}
	if r.has(24) {
		if m.BlackProfile, err = decodePlayer(rc); err != nil {
			return nil, err
		}
	}
	for _, p := range []*Player{m.WhiteProfile, m.BlackProfile} {
		if p != nil && p.UserID != "" && !r.has(21) {
			return nil, rc.errorf("user_id without site")
		}
	}
	m.Rated = r.has(25)
	return m, r.finish(26)
}

func isPow2(v uint64) bool { return v != 0 && v&(v-1) == 0 }

func (p *parser) decodeGame(c *cursor) (*Game, error) {
	r, err := c.record()
	if err != nil {
		return nil, err
	}
	g := &Game{InitialCubeValue: 1, InitialCubeOwner: Centred}
	rc := &r.c
	if r.has(0) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		s := Seat(v)
		g.Winner = &s
	}
	if r.has(1) {
		if g.PointsWon, err = intp(rc); err != nil {
			return nil, err
		}
	}
	g.IsLastGame = r.has(2)
	if r.has(3) {
		if g.InitialBoard, err = board(rc); err != nil {
			return nil, err
		}
		// A default is never written (3.2): the opening board is the absent field.
		if open, ok := openings[p.f.Match.Variant]; ok && *g.InitialBoard == open {
			return nil, rc.errorf("initial_board written at the opening position")
		}
	}
	if r.has(4) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		if v == 1 || !isPow2(v) {
			return nil, rc.errorf("initial_cube_value %d", v)
		}
		g.InitialCubeValue = int(v)
	}
	if r.has(5) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		if Seat(v) == Centred {
			return nil, rc.errorf("initial_cube_owner written at its default")
		}
		g.InitialCubeOwner = Seat(v)
	}
	if r.has(6) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		if v == 0 {
			return nil, rc.errorf("auto_doubles 0 written")
		}
		g.AutoDoubles = int(v)
	}
	if r.has(7) {
		if g.Termination, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if err := r.finish(8); err != nil {
		return nil, err
	}
	for c.remaining() > 0 {
		if len(g.Plies) >= MaxPliesPerGame || p.plyCount >= MaxTotalPlies {
			return nil, c.errorf("too many plies")
		}
		ply, err := decodePly(c)
		if err != nil {
			return nil, err
		}
		ply.Ref = p.plyCount
		p.plyCount++
		g.Plies = append(g.Plies, *ply)
	}
	return g, nil
}

func decodePly(c *cursor) (*Ply, error) {
	b0, err := c.u8()
	if err != nil {
		return nil, err
	}
	p := &Ply{Action: int(b0 & 0x3f), Seat: Seat((b0 >> 6) & 1)}
	hasExtras := b0&0x80 != 0
	if p.IsDiceAction() {
		n := 2
		if d := diceOfAction[p.Action]; d[0] == d[1] {
			n = 4
		}
		for i := 0; i < n; i++ {
			b, err := c.u8()
			if err != nil {
				return nil, err
			}
			if b == 0 {
				break
			}
			s, err := step(b)
			if err != nil {
				return nil, c.errorf("invalid step byte %#x", b)
			}
			p.Steps = append(p.Steps, s)
		}
	}
	if p.Action == ActionReserved {
		return nil, c.errorf("reserved action 35")
	}
	if !hasExtras {
		if p.Action == actionEscape {
			return nil, c.errorf("escape action without extras")
		}
		return p, checkPlyFields(c, p)
	}
	r, err := c.record()
	if err != nil {
		return nil, err
	}
	if r.mask == 0 {
		return nil, c.errorf("has_extras with an empty presence mask")
	}
	rc := &r.c
	if r.has(0) {
		var d [2]int
		for i := range d {
			v, err := rc.u8()
			if err != nil {
				return nil, err
			}
			if v < 1 || v > 6 {
				return nil, rc.errorf("die %d", v)
			}
			d[i] = int(v)
		}
		p.ExtraDice = &d
	}
	if r.has(1) {
		if p.ResignValue, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if r.has(2) {
		if p.CubeValue, err = intp(rc); err != nil {
			return nil, err
		}
	}
	p.Illegal = r.has(3)
	if r.has(4) {
		if p.SettleValue, err = equity(rc); err != nil {
			return nil, err
		}
	}
	var extraSteps bool
	if r.has(5) {
		if p.IsDiceAction() {
			return nil, rc.errorf("extras steps on a dice action")
		}
		if p.Steps, err = countedSteps(rc); err != nil {
			return nil, err
		}
		if len(p.Steps) == 0 {
			return nil, rc.errorf("empty extras steps")
		}
		extraSteps = true
	}
	if r.has(6) {
		if p.Board, err = board(rc); err != nil {
			return nil, err
		}
	}
	if r.has(7) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		if p.Action != actionEscape || v < 64 {
			return nil, rc.errorf("action_ext %d", v)
		}
		p.Action = int(v)
	} else if p.Action == actionEscape {
		return nil, rc.errorf("escape action without action_ext")
	}
	if r.has(8) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		s := Seat(v)
		p.CubeOwner = &s
	}
	if err := r.finish(9); err != nil {
		return nil, err
	}
	_ = extraSteps
	return p, checkPlyFields(c, p)
}

// checkPlyFields enforces which extras each assigned action carries (5.1).
func checkPlyFields(c *cursor, p *Ply) error {
	if p.Action > ActionPass {
		return nil // unknown action: not checked (P2)
	}
	isMarker := p.Action == ActionGameOver || p.Action == ActionMatchOver || p.Action == ActionFinal || p.Action == ActionNull
	if isMarker && p.Seat != White {
		return c.errorf("marker with seat bit 1")
	}
	resign := p.Action == ActionResignGame || p.Action == ActionResignMatch
	if (p.ResignValue != nil) != resign {
		return c.errorf("resign_value on action %d", p.Action)
	}
	if (p.CubeValue != nil) != (p.Action == ActionCubeSet) {
		return c.errorf("cube_value on action %d", p.Action)
	}
	if p.CubeValue != nil && !isPow2(uint64(*p.CubeValue)) {
		return c.errorf("cube_value %d is not a power of two", *p.CubeValue)
	}
	if p.CubeOwner != nil && p.Action != ActionCubeSet {
		return c.errorf("cube_owner on action %d", p.Action)
	}
	if (p.SettleValue != nil) != (p.Action == ActionSettle) {
		return c.errorf("settle_value on action %d", p.Action)
	}
	if (p.Board != nil) != (p.Action == ActionSetPosition) {
		return c.errorf("board on action %d", p.Action)
	}
	if (p.ExtraDice != nil || p.Illegal) && p.Action != ActionSetPosition {
		return c.errorf("dice or illegal on action %d", p.Action)
	}
	if p.Illegal && (p.ExtraDice == nil || p.Board == nil) {
		return c.errorf("illegal without dice and board")
	}
	return nil
}

func decodeRollout(c *cursor) (*Rollout, error) {
	r, err := c.record()
	if err != nil {
		return nil, err
	}
	ro := &Rollout{}
	rc := &r.c
	fields := []**int{&ro.Trials, &ro.TruncationDepth, &ro.MovePly, &ro.VarianceReduction}
	for bit, dst := range fields {
		if r.has(uint(bit)) {
			if *dst, err = intp(rc); err != nil {
				return nil, err
			}
		}
	}
	if r.has(4) {
		v, err := rc.u64()
		if err != nil {
			return nil, err
		}
		ro.Seed = &v
	}
	if r.has(5) {
		if ro.BudgetMS, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if r.has(6) {
		if ro.MatchPolicy, err = intp(rc); err != nil {
			return nil, err
		}
	}
	return ro, r.finish(7)
}

func decodeLevel(c *cursor) (*Level, error) {
	r, err := c.record()
	if err != nil {
		return nil, err
	}
	if r.mask == 0 {
		return nil, c.errorf("level with no field")
	}
	l := &Level{}
	rc := &r.c
	if r.has(0) {
		s, err := rc.str()
		if err != nil {
			return nil, err
		}
		l.Preset = &s
	}
	if r.has(1) {
		if l.CheckerPly, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if r.has(2) {
		if l.CubePly, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if r.has(3) {
		if l.Rollout, err = decodeRollout(rc); err != nil {
			return nil, err
		}
	}
	l.NoRollout = r.has(4)
	if l.NoRollout && l.Rollout != nil {
		return nil, rc.errorf("rollout and no_rollout together")
	}
	return l, r.finish(5)
}

// Effective resolves a level field by field against the tier above (6.4).
func (l *Level) Effective(parent *Level) *Level {
	if l == nil {
		return parent
	}
	if parent == nil {
		parent = &Level{}
	}
	out := *parent
	if l.Preset != nil {
		out.Preset = l.Preset
	}
	if l.CheckerPly != nil {
		out.CheckerPly = l.CheckerPly
	}
	if l.CubePly != nil {
		out.CubePly = l.CubePly
	}
	switch {
	case l.NoRollout:
		out.Rollout = nil
	case l.Rollout != nil:
		ro := Rollout{}
		if parent.Rollout != nil {
			ro = *parent.Rollout
		}
		mergeRollout(&ro, l.Rollout)
		out.Rollout = &ro
	}
	out.NoRollout = false
	return &out
}

func mergeRollout(dst, src *Rollout) {
	pick := func(d **int, s *int) {
		if s != nil {
			*d = s
		}
	}
	pick(&dst.Trials, src.Trials)
	pick(&dst.TruncationDepth, src.TruncationDepth)
	pick(&dst.MovePly, src.MovePly)
	pick(&dst.VarianceReduction, src.VarianceReduction)
	pick(&dst.BudgetMS, src.BudgetMS)
	pick(&dst.MatchPolicy, src.MatchPolicy)
	if src.Seed != nil {
		dst.Seed = src.Seed
	}
}

func decodeAnalysis(c *cursor) (*Analysis, error) {
	r, err := c.record()
	if err != nil {
		return nil, err
	}
	if c.remaining() != 0 {
		return nil, c.errorf("trailing bytes after ANAL record")
	}
	a := &Analysis{Currency: CurrencyUnset}
	rc := &r.c
	if a.ID, err = identifier(rc); err != nil {
		return nil, err
	}
	if r.has(0) {
		if a.MatchDigest, err = digest(rc); err != nil {
			return nil, err
		}
	}
	if r.has(1) {
		if a.Producer, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if r.has(2) {
		if a.Level, err = decodeLevel(rc); err != nil {
			return nil, err
		}
	}
	a.Complete = r.has(3)
	if r.has(4) {
		n, err := rc.v32()
		if err != nil {
			return nil, err
		}
		if n > MaxCoverage {
			return nil, rc.errorf("coverage of %d entries", n)
		}
		a.Coverage = make([]int, 0, n)
		prev := -1
		for i := 0; i < int(n); i++ {
			v, err := rc.v32()
			if err != nil {
				return nil, err
			}
			ref := int(v)
			if i > 0 {
				ref = prev + 1 + int(v)
			}
			a.Coverage = append(a.Coverage, ref)
			prev = ref
		}
	}
	strs := []struct {
		bit uint
		dst *string
	}{{5, &a.ModelID}, {6, &a.ModelName}}
	for _, s := range strs {
		if r.has(s.bit) {
			if *s.dst, err = rc.str(); err != nil {
				return nil, err
			}
		}
	}
	if r.has(7) {
		if a.ModelDigest, err = digest(rc); err != nil {
			return nil, err
		}
	}
	if r.has(8) {
		if a.EngineBuild, err = rc.str(); err != nil {
			return nil, err
		}
	}
	if r.has(9) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		a.Currency = Currency(v)
	}
	if r.has(10) {
		x, err := probability(rc)
		if err != nil {
			return nil, err
		}
		a.CubeEfficiency = &x
	}
	if r.has(11) {
		if a.METID, err = rc.str(); err != nil {
			return nil, err
		}
	}
	if r.has(12) {
		if a.Tables, err = rc.str(); err != nil {
			return nil, err
		}
	}
	if r.has(13) {
		if _, err := rc.record(); err != nil { // dials: provenance, not decoded
			return nil, err
		}
	}
	if r.has(14) {
		if a.StartedAt, err = instant(rc); err != nil {
			return nil, err
		}
	}
	if r.has(15) {
		if a.CompletedAt, err = instant(rc); err != nil {
			return nil, err
		}
	}
	if r.has(16) {
		if a.DurationMS, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if r.has(17) {
		n, err := rc.v32()
		if err != nil {
			return nil, err
		}
		if n == 0 || n > MaxAnalyses*64 {
			return nil, rc.errorf("sources count %d", n)
		}
		for i := 0; i < int(n); i++ {
			id, err := identifier(rc)
			if err != nil {
				return nil, err
			}
			a.Sources = append(a.Sources, id)
		}
	}
	return a, r.finish(18)
}

func producerRef(rc *cursor, a *Analysis) (*int, error) {
	v, err := intp(rc)
	if err != nil {
		return nil, err
	}
	if *v >= len(a.Sources) {
		return nil, rc.errorf("producer_ref %d past the block's sources", *v)
	}
	return v, nil
}

func (p *parser) decodeDecisions(c *cursor, a *Analysis) error {
	covered := map[int]bool{}
	for _, ref := range a.Coverage {
		covered[ref] = true
	}
	lastRef, lastKind := -1, -1
	for c.remaining() > 0 {
		if len(a.Decisions) >= MaxDecisions {
			return c.errorf("too many decisions")
		}
		r, err := c.record()
		if err != nil {
			return err
		}
		rc := &r.c
		ref, err := rc.v32()
		if err != nil {
			return err
		}
		kind, err := rc.v32()
		if err != nil {
			return err
		}
		if ref >= MaxTotalPlies {
			return rc.errorf("ply_ref %d", ref)
		}
		if int(ref) < lastRef || (int(ref) == lastRef && int(kind) <= lastKind) {
			return rc.errorf("decisions not in ascending (ply_ref, kind) order")
		}
		lastRef, lastKind = int(ref), int(kind)
		if a.Coverage != nil && !covered[int(ref)] {
			return rc.errorf("decision at ply %d outside coverage", ref)
		}
		d := Decision{PlyRef: int(ref), Kind: DecisionKind(kind)}
		switch d.Kind {
		case KindChecker:
			d.Checker, err = decodeChecker(r, a)
		case KindCube:
			d.Cube, err = decodeCube(r, a)
		case KindResign:
			d.Resign, err = decodeResign(r, a)
		case KindRoll:
			d.Roll, err = decodeRoll(r, a)
		}
		if err != nil {
			return err
		}
		a.Decisions = append(a.Decisions, d)
	}
	return nil
}

func decodeChecker(r *record, a *Analysis) (*CheckerDecision, error) {
	if r.mask == 0 {
		return nil, r.c.errorf("CHECKER with no field")
	}
	d := &CheckerDecision{}
	rc := &r.c
	var err error
	if r.has(0) {
		n, err := rc.v32()
		if err != nil {
			return nil, err
		}
		if n == 0 || n > MaxAltsPerDecision {
			return nil, rc.errorf("%d alternatives", n)
		}
		played := 0
		for i := 0; i < int(n); i++ {
			alt, err := decodeAlternative(rc)
			if err != nil {
				return nil, err
			}
			if alt.IsPlayed {
				played++
			}
			d.Alternatives = append(d.Alternatives, *alt)
		}
		if played > 1 {
			return nil, rc.errorf("more than one played alternative")
		}
	}
	if r.has(1) {
		if d.AlternativesTotal, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if r.has(2) {
		if r.has(0) {
			return nil, rc.errorf("best_equity stored beside alternatives")
		}
		if d.BestEquity, err = equity(rc); err != nil {
			return nil, err
		}
	}
	if r.has(3) {
		if d.EquityLoss, err = equityLoss(rc); err != nil {
			return nil, err
		}
	}
	if r.has(4) {
		if d.Level, err = decodeLevel(rc); err != nil {
			return nil, err
		}
	}
	for _, bit := range []uint{5, 6} {
		if r.has(bit) {
			if _, err := rc.v32(); err != nil {
				return nil, err
			}
		}
	}
	if r.has(7) {
		v, err := rc.varint(64)
		if err != nil {
			return nil, err
		}
		d.PositionTags = &v
	}
	if r.has(8) {
		if d.ProducerRef, err = producerRef(rc, a); err != nil {
			return nil, err
		}
	}
	if r.has(9) {
		if _, err := rc.v32(); err != nil {
			return nil, err
		}
	}
	return d, r.finish(10)
}

func decodeAlternative(c *cursor) (*Alternative, error) {
	r, err := c.record()
	if err != nil {
		return nil, err
	}
	rc := &r.c
	alt := &Alternative{}
	if alt.Steps, err = countedSteps(rc); err != nil {
		return nil, err
	}
	e, err := equity(rc)
	if err != nil {
		return nil, err
	}
	alt.Equity = *e
	if r.has(0) {
		if alt.Probs, err = probabilities(rc); err != nil {
			return nil, err
		}
	}
	if r.has(1) {
		if alt.Level, err = decodeLevel(rc); err != nil {
			return nil, err
		}
	}
	alt.IsPlayed = r.has(2)
	if r.has(3) {
		if alt.RolloutSE, err = equity(rc); err != nil {
			return nil, err
		}
	}
	if r.has(4) {
		if alt.CubelessEquity, err = equity(rc); err != nil {
			return nil, err
		}
	}
	return alt, r.finish(5)
}

func decodeCube(r *record, a *Analysis) (*CubeDecision, error) {
	rc := &r.c
	v, err := rc.v32()
	if err != nil {
		return nil, err
	}
	d := &CubeDecision{Verdict: Verdict(v)}
	eqs := []**float64{&d.NoDoubleEquity, &d.DoubleTakeEquity, &d.DoublePassEquity}
	for bit, dst := range eqs {
		if r.has(uint(bit)) {
			if *dst, err = equity(rc); err != nil {
				return nil, err
			}
		}
	}
	if r.has(3) {
		if d.Probs, err = probabilities(rc); err != nil {
			return nil, err
		}
	}
	if r.has(4) {
		if d.EquityLoss, err = equityLoss(rc); err != nil {
			return nil, err
		}
	}
	if r.has(5) {
		x, err := probability(rc)
		if err != nil {
			return nil, err
		}
		d.TakePoint = &x
	}
	d.WindowSearched = r.has(6)
	if r.has(7) {
		if d.Level, err = decodeLevel(rc); err != nil {
			return nil, err
		}
	}
	d.IsOptional = r.has(8)
	d.IsFreeCube = r.has(9)
	if r.has(10) {
		if d.CubefulTakeValue, err = equity(rc); err != nil {
			return nil, err
		}
	}
	if r.has(11) {
		v, err := rc.v32()
		if err != nil {
			return nil, err
		}
		cur := Currency(v)
		if cur == a.Currency {
			return nil, rc.errorf("CUBE currency equal to the block's")
		}
		d.Currency = &cur
	}
	if r.has(12) {
		if d.ProducerRef, err = producerRef(rc, a); err != nil {
			return nil, err
		}
	}
	return d, r.finish(13)
}

func decodeResign(r *record, a *Analysis) (*ResignDecision, error) {
	if r.mask == 0 {
		return nil, r.c.errorf("RESIGN with no field")
	}
	rc := &r.c
	d := &ResignDecision{}
	var err error
	if r.has(0) {
		if d.CorrectValue, err = intp(rc); err != nil {
			return nil, err
		}
	}
	if r.has(1) {
		if d.ResignError, err = equity(rc); err != nil {
			return nil, err
		}
	}
	if r.has(2) {
		if d.TakeResignError, err = equity(rc); err != nil {
			return nil, err
		}
	}
	if r.has(3) {
		if d.Probs, err = probabilities(rc); err != nil {
			return nil, err
		}
	}
	if r.has(4) {
		if d.EquityLoss, err = equityLoss(rc); err != nil {
			return nil, err
		}
	}
	if r.has(5) {
		if d.Level, err = decodeLevel(rc); err != nil {
			return nil, err
		}
	}
	if r.has(6) {
		if d.ProducerRef, err = producerRef(rc, a); err != nil {
			return nil, err
		}
	}
	return d, r.finish(7)
}

func decodeRoll(r *record, a *Analysis) (*RollDecision, error) {
	rc := &r.c
	luck, err := equity(rc)
	if err != nil {
		return nil, err
	}
	d := &RollDecision{Luck: *luck}
	if r.has(0) {
		if d.Level, err = decodeLevel(rc); err != nil {
			return nil, err
		}
	}
	if r.has(1) {
		if d.ProducerRef, err = producerRef(rc, a); err != nil {
			return nil, err
		}
	}
	return d, r.finish(2)
}

func decodeSignature(c *cursor, msig bool) (*Signature, error) {
	r, err := c.record()
	if err != nil {
		return nil, err
	}
	if c.remaining() != 0 {
		return nil, c.errorf("trailing bytes after signature record")
	}
	rc := &r.c
	alg, err := rc.v32()
	if err != nil {
		return nil, err
	}
	if alg == 0 {
		return nil, rc.errorf("signature algorithm 0")
	}
	s := &Signature{Algorithm: int(alg)}
	if s.Signature, err = rc.blob(); err != nil {
		return nil, err
	}
	if r.has(0) {
		if s.KeyID, err = rc.str(); err != nil {
			return nil, err
		}
	}
	if r.has(1) {
		if s.PublicKey, err = rc.blob(); err != nil {
			return nil, err
		}
	}
	if r.has(2) {
		if s.SignedAt, err = instant(rc); err != nil {
			return nil, err
		}
	}
	known := uint(3)
	if msig {
		known = 4
		if r.has(3) {
			v, err := rc.varint(64)
			if err != nil {
				return nil, err
			}
			if v == 0 {
				return nil, rc.errorf("covers 0 written")
			}
			s.Covers = v
		}
	}
	return s, r.finish(known)
}

func decodeAnnotations(c *cursor) ([]Annotation, error) {
	out := []Annotation{}
	for c.remaining() > 0 {
		if len(out) >= MaxAnnotations {
			return nil, c.errorf("too many annotations")
		}
		r, err := c.record()
		if err != nil {
			return nil, err
		}
		rc := &r.c
		var a Annotation
		scope, err := rc.v32()
		if err != nil {
			return nil, err
		}
		ref, err := rc.v32()
		if err != nil {
			return nil, err
		}
		a.Scope, a.Ref = int(scope), int(ref)
		if a.Value, err = rc.str(); err != nil {
			return nil, err
		}
		if r.has(0) {
			k, err := rc.str()
			if err != nil {
				return nil, err
			}
			a.Key = &k
		}
		if r.has(1) {
			v, err := rc.v32()
			if err != nil {
				return nil, err
			}
			k := DecisionKind(v)
			a.Kind = &k
		}
		if r.has(2) {
			if a.AltIndex, err = intp(rc); err != nil {
				return nil, err
			}
		}
		if r.has(3) {
			if a.Lang, err = rc.str(); err != nil {
				return nil, err
			}
		}
		if r.has(4) {
			if a.Author, err = rc.str(); err != nil {
				return nil, err
			}
		}
		if r.has(5) {
			if a.At, err = instant(rc); err != nil {
				return nil, err
			}
		}
		if r.has(6) {
			n, err := rc.v32()
			if err != nil {
				return nil, err
			}
			if n == 0 || n > MaxDrawings {
				return nil, rc.errorf("%d drawings", n)
			}
			for i := 0; i < int(n); i++ {
				if _, err := rc.record(); err != nil {
					return nil, err
				}
			}
			a.Drawings = int(n)
		}
		if r.has(7) {
			id, err := identifier(rc)
			if err != nil {
				return nil, err
			}
			a.Analysis = &id
		}
		if err := r.finish(8); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
