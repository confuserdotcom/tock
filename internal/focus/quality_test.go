package focus

import "testing"

// q50 is a 50 min manual session: share 82, longest 70, calm 50, steady 36.
func q50() Session {
	return Session{Kind: KindManual, StartMS: 0, EndMS: 50 * minute, PlannedMS: 50 * minute,
		DeepMS: 41 * minute, LongestMS: 35 * minute, Interruptions: 1, Switches: 3}
}

func values(q QualityScore) [4]int {
	var v [4]int
	for i, p := range q.Parts {
		v[i] = p.Value
	}
	return v
}

func TestQualityParts(t *testing.T) {
	tests := []struct {
		name  string
		s     Session
		parts [4]int
		score int
	}{
		{"50 min", q50(), [4]int{82, 70, 50, 36}, 67},
		{"perfect 25 min", Session{Kind: KindManual, EndMS: 25 * minute, DeepMS: 25 * minute, LongestMS: 25 * minute},
			[4]int{100, 100, 100, 100}, 100},
		{"90 min rounds half up", Session{Kind: KindManual, EndMS: 90 * minute, DeepMS: 60 * minute,
			LongestMS: 50 * minute, Interruptions: 3, Switches: 6}, [4]int{67, 100, 25, 33}, 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Quality(tt.s, DefaultQualityWeights())
			if v := values(q); v != tt.parts || q.Score != tt.score {
				t.Fatalf("parts %v score %d, want %v score %d", v, q.Score, tt.parts, tt.score)
			}
		})
	}
	if a, b := Quality(q50(), Weights{}), Quality(q50(), DefaultQualityWeights()); a != b {
		t.Fatalf("zero weights = %+v, want the defaults %+v", a, b)
	}
}

func TestQualityLegacySteady(t *testing.T) {
	s := q50()
	s.Switches = -1
	q := Quality(s, Weights{})
	if q.Score != 73 || !q.Parts[3].NA || q.Parts[3].Value != 0 {
		t.Fatalf("legacy = %+v, want score 73 and steady n/a", q)
	}
	if got := values(q); got[0] != 82 || got[1] != 70 || got[2] != 50 {
		t.Fatalf("other parts = %v", got)
	}
}

func TestQualityZeroLength(t *testing.T) {
	for _, kind := range []string{KindManual, KindAuto} {
		q := Quality(Session{Kind: kind, StartMS: 5 * minute, EndMS: 5 * minute, DeepMS: minute, Interruptions: 2, Switches: 4}, Weights{})
		if q.Score != 0 || values(q) != [4]int{} {
			t.Errorf("%s zero length = %+v, want all 0", kind, q)
		}
	}
}

func TestQualityEmptyWindow(t *testing.T) {
	q := Quality(Session{Kind: KindManual, EndMS: 30 * minute, PlannedMS: 30 * minute}, Weights{})
	if values(q) != [4]int{0, 0, 100, 100} || q.Score != 30 {
		t.Fatalf("empty window = %+v, want parts 0 0 100 100 score 30", q)
	}
}

func TestQualityAutoCalm(t *testing.T) {
	a := Session{Kind: KindAuto, EndMS: 40 * minute, DeepMS: 40 * minute, LongestMS: 40 * minute, Interruptions: 5, Switches: 0}
	b := a
	b.Interruptions = 0
	qa, qb := Quality(a, Weights{}), Quality(b, Weights{})
	if qa != qb {
		t.Fatalf("interruptions changed an auto score: %+v vs %+v", qa, qb)
	}
	if p := qa.Parts[2]; p.Value != 100 || !p.Auto || qa.Score != 100 {
		t.Fatalf("auto calm = %+v score %d, want 100 / Auto / 100", p, qa.Score)
	}
}

func TestQualityClamp(t *testing.T) {
	q := Quality(Session{Kind: KindManual, EndMS: 10 * minute, DeepMS: 30 * minute, LongestMS: 30 * minute}, Weights{})
	if q.Parts[0].Value != 100 || q.Parts[1].Value != 100 || q.Score > 100 {
		t.Fatalf("clamp = %+v", q)
	}
	q = Quality(Session{Kind: KindManual, EndMS: 10 * minute}, Weights{Share: 3, Longest: 3, Calm: 3, Steady: 3})
	if q.Score != 100 {
		t.Fatalf("score with big weights = %d, want clamped to 100", q.Score)
	}
}

func TestQualityLine(t *testing.T) {
	s := q50()
	if got, want := Quality(s, Weights{}).Line(), "share 82 · longest 70 · calm 50 · steady 36"; got != want {
		t.Errorf("Line = %q, want %q", got, want)
	}
	s.Switches = -1
	if got, want := Quality(s, Weights{}).Line(), "share 82 · longest 70 · calm 50 · steady n/a"; got != want {
		t.Errorf("legacy Line = %q, want %q", got, want)
	}
	s = q50()
	s.Kind = KindAuto
	if got, want := Quality(s, Weights{}).Line(), "share 82 · longest 70 · calm n/a (auto) · steady 36"; got != want {
		t.Errorf("auto Line = %q, want %q", got, want)
	}
}

func TestInterruptionsText(t *testing.T) {
	for n, want := range map[int]string{0: "no interruptions", 1: "1 interruption", 3: "3 interruptions"} {
		if got := InterruptionsText(n); got != want {
			t.Errorf("InterruptionsText(%d) = %q, want %q", n, got, want)
		}
	}
}
