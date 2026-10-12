package vote

import "testing"

func everyone(string) bool { return true }

func TestNeeded(t *testing.T) {
	for listeners, want := range map[int]int{0: 1, 1: 1, 2: 2, 3: 2, 4: 3, 5: 3, 10: 6} {
		if got := Needed(listeners); got != want {
			t.Errorf("Needed(%d) = %d, want %d", listeners, got, want)
		}
	}
}

func TestCastPassesOnMajority(t *testing.T) {
	b := NewBox()
	if r := b.Cast("g", Skip, "a", everyone, 2); !r.Added || r.Passed || r.Count != 1 || r.Needed != 2 {
		t.Fatalf("first vote: %+v", r)
	}
	if b.Count("g", Disconnect, everyone) != 0 || b.Count("other", Skip, everyone) != 0 {
		t.Fatal("polls must be separated by guild and kind")
	}
	if r := b.Cast("g", Skip, "b", everyone, 2); !r.Passed || r.Count != 2 {
		t.Fatalf("second vote must pass: %+v", r)
	}
	if b.Count("g", Skip, everyone) != 0 {
		t.Fatal("passed poll must be closed")
	}
}

func TestCastTwiceWithdraws(t *testing.T) {
	b := NewBox()
	b.Cast("g", Disconnect, "a", everyone, 3)
	if r := b.Cast("g", Disconnect, "a", everyone, 3); r.Added || r.Passed || r.Count != 0 {
		t.Fatalf("repeated vote must be withdrawn: %+v", r)
	}
}

func TestOnlyEligibleVotesCount(t *testing.T) {
	b := NewBox()
	present := map[string]bool{"a": true, "b": true}
	eligible := func(id string) bool { return present[id] }
	b.Cast("g", Skip, "a", eligible, 2)
	delete(present, "a")
	if r := b.Cast("g", Skip, "b", eligible, 2); r.Passed || r.Count != 1 {
		t.Fatalf("votes of users who left must not count: %+v", r)
	}
}

func TestCheckPassesWhenListenersLeave(t *testing.T) {
	b := NewBox()
	b.Cast("g", Skip, "a", everyone, 2)
	if b.Check("g", Skip, everyone, 2) {
		t.Fatal("poll below the threshold must stay open")
	}
	if !b.Check("g", Skip, everyone, 1) {
		t.Fatal("poll must pass once fewer votes are needed")
	}
	if b.Check("g", Skip, everyone, 1) {
		t.Fatal("passed poll must be closed")
	}
}

func TestClear(t *testing.T) {
	b := NewBox()
	b.Cast("g", Skip, "a", everyone, 3)
	b.Cast("g", Disconnect, "a", everyone, 3)
	b.Cast("h", Skip, "a", everyone, 3)
	b.Clear("g")
	if b.Count("g", Skip, everyone) != 0 || b.Count("g", Disconnect, everyone) != 0 || b.Count("h", Skip, everyone) != 1 {
		t.Fatal("clear must drop only the guild polls")
	}
}
