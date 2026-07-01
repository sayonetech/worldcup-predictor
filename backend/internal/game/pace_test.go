package game

import "testing"

func TestPaceDistance_WorkedTable(t *testing.T) {
	cases := []struct {
		ms   float64
		want int
	}{
		{1000, 23}, {2000, 46}, {5000, 119}, {10000, 246}, {15000, 380},
		{20000, 522}, {30000, 829}, {60000, 1930}, {90000, 3303},
		{120000, 4939}, {180000, 8454}, {300000, 16357}, {600000, 42306},
	}
	for _, c := range cases {
		if got := PaceDistance(c.ms); got != c.want {
			t.Errorf("PaceDistance(%v) = %d, want %d", c.ms, got, c.want)
		}
	}
}

func TestPaceDistance_NonPositive(t *testing.T) {
	if PaceDistance(0) != 0 || PaceDistance(-5) != 0 {
		t.Fatal("non-positive activeMs must yield 0")
	}
}

func TestPaceDistance_Monotonic(t *testing.T) {
	prev := -1
	for ms := 0.0; ms <= 600000; ms += 250 {
		d := PaceDistance(ms)
		if d < prev {
			t.Fatalf("distance decreased at %vms: %d < %d", ms, d, prev)
		}
		prev = d
	}
}
