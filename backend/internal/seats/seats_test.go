package seats

import (
	"slices"
	"testing"
)

func TestSeatLabel(t *testing.T) {
	cases := map[int]string{1: "A1", 10: "A10", 11: "B1", 45: "E5", 80: "H10"}
	for id, want := range cases {
		if got := SeatLabel(id); got != want {
			t.Errorf("SeatLabel(%d) = %s, want %s", id, got, want)
		}
	}
}

func TestValidateSeatSelection(t *testing.T) {
	got, err := ValidateSeatSelection([]int{12, 3, 7})
	if err != nil || !slices.Equal(got, []int{3, 7, 12}) {
		t.Fatalf("got %v, %v; want sorted [3 7 12]", got, err)
	}
	for name, ids := range map[string][]int{
		"empty":     {},
		"seven":     {1, 2, 3, 4, 5, 6, 7},
		"duplicate": {4, 4},
		"zero":      {0},
		"too high":  {81},
	} {
		if _, err := ValidateSeatSelection(ids); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := ValidateSeatSelection([]int{1, 2, 3, 4, 5, 6}); err != nil {
		t.Errorf("six seats should be allowed: %v", err)
	}
}

func TestValidUUID(t *testing.T) {
	if !validUUID("c89ede66-4fcf-492c-af9c-27a4fbde9b9a") {
		t.Error("valid uuid rejected")
	}
	for _, s := range []string{"", "1", "c89ede66-4fcf-492c-af9c-27a4fbde9b9", "c89ede66x4fcf-492c-af9c-27a4fbde9b9a", "zzzzzzzz-4fcf-492c-af9c-27a4fbde9b9a"} {
		if validUUID(s) {
			t.Errorf("invalid uuid %q accepted", s)
		}
	}
}
