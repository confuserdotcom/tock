package focus

import (
	"testing"

	"tock/internal/config"
)

func levelOf(from, now int64, vs ...vw) Live {
	views, labels := world(vs...)
	return Level(views, labels, from, now, config.DefaultFocus())
}

func TestLevelRecoversAfterADrop(t *testing.T) {
	from := at(9, 0, 0)
	vs := []vw{
		{from, from + 2*minute, "deep", "", ""},
		{from + 2*minute, from + 6*minute, "distract", "", ""},
		{from + 6*minute, from + 30*minute, "deep", "", ""},
	}
	dropped := levelOf(from, from+6*minute, vs...)
	if dropped.Level > 50 {
		t.Fatalf("level right after 4m of distraction = %d, want ≤ 50", dropped.Level)
	}
	if dropped.Trend >= 0 {
		t.Fatalf("trend while distracted = %d, want < 0", dropped.Trend)
	}
	back := levelOf(from, from+30*minute, vs...)
	if back.Level < 90 {
		t.Fatalf("level after 24m of deep work = %d, want ≥ 90", back.Level)
	}
	// The plain share would still be 26/30 = 87%: the level is higher.
	if back.Trend < 0 {
		t.Fatalf("trend while recovering = %d, want ≥ 0", back.Trend)
	}
}

func TestLevelForgivesABlip(t *testing.T) {
	from := at(9, 0, 0)
	got := levelOf(from, from+20*minute,
		vw{from, from + 10*minute, "deep", "", ""},
		vw{from + 10*minute, from + 10*minute + 40_000, "distract", "", ""},
		vw{from + 10*minute + 40_000, from + 20*minute, "deep", "", ""})
	if got.Level != 100 {
		t.Fatalf("level with a 40 s blip between deep work = %d, want 100", got.Level)
	}
}

func TestLevelOpenBlipCountsUntilDeepWorkResumes(t *testing.T) {
	from := at(9, 0, 0)
	got := levelOf(from, from+10*minute+30_000,
		vw{from, from + 10*minute, "deep", "", ""},
		vw{from + 10*minute, from + 10*minute + 30_000, "distract", "", ""})
	if got.Level >= 100 {
		t.Fatalf("level during a distraction still going = %d, want < 100", got.Level)
	}
}

func TestLevelIdleIsNotForgiven(t *testing.T) {
	from := at(9, 0, 0)
	got := levelOf(from, from+20*minute,
		vw{from, from + 10*minute, "deep", "", ""},
		vw{from + 10*minute, from + 10*minute + 40_000, "idle", "", ""},
		vw{from + 10*minute + 40_000, from + 20*minute, "deep", "", ""})
	if got.Level == 100 {
		t.Fatal("idle between deep work was forgiven")
	}
}

func TestLevelAllDistraction(t *testing.T) {
	from := at(9, 0, 0)
	if got := levelOf(from, from+10*minute, vw{from, from + 10*minute, "distract", "", ""}); got.Level != 0 {
		t.Fatalf("level = %d, want 0", got.Level)
	}
}

func TestLevelSkipsMeetingsAndEmpty(t *testing.T) {
	from := at(9, 0, 0)
	if got := levelOf(from, from); got.Level != -1 {
		t.Fatalf("empty level = %d, want -1", got.Level)
	}
	got := levelOf(from, from+20*minute,
		vw{from, from + 10*minute, "deep", "", ""},
		vw{from + 10*minute, from + 20*minute, "distract", "", "meet.google.com/abc"})
	if got.Level != 100 {
		t.Fatalf("level with a meeting after deep work = %d, want 100", got.Level)
	}
}

func TestLevelShallowIsHalf(t *testing.T) {
	from := at(9, 0, 0)
	if got := levelOf(from, from+10*minute, vw{from, from + 10*minute, "shallow", "", ""}); got.Level != 50 {
		t.Fatalf("level = %d, want 50", got.Level)
	}
}
