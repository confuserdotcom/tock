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
