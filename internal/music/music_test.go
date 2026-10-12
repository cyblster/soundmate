package music

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgolink/v3/lavalink"
)

func tracks(names ...string) []Track {
	result := make([]Track, len(names))
	for i, n := range names {
		result[i] = Track{Title: n, Encoded: "enc-" + n}
	}
	return result
}

func titles(ts []Track) []string {
	result := make([]string, len(ts))
	for i, t := range ts {
		result[i] = t.Title
	}
	return result
}

func TestEnqueueStartsIdlePlayer(t *testing.T) {
	var p Player
	start := p.Enqueue(tracks("a", "b")...)
	if start == nil || start.Title != "a" {
		t.Fatalf("Enqueue() on idle player = %v, want track a", start)
	}
	if c := p.Current(); c == nil || c.Title != "a" {
		t.Fatalf("Current() = %v, want a", c)
	}
	s := p.Snapshot(-1)
	if s.Total != 1 || !slices.Equal(titles(s.Upcoming), []string{"b"}) {
		t.Fatalf("Upcoming = %v (%d), want [b]", titles(s.Upcoming), s.Total)
	}
}

func TestEnqueueOnActivePlayerOnlyAppends(t *testing.T) {
	var p Player
	p.Enqueue(tracks("a")...)
	if start := p.Enqueue(tracks("b", "c")...); start != nil {
		t.Fatalf("Enqueue() on active player = %v, want nil", start)
	}
	if got := titles(p.Snapshot(-1).Upcoming); !slices.Equal(got, []string{"b", "c"}) {
		t.Fatalf("Upcoming = %v, want [b c]", got)
	}
}

func TestEnqueueNothing(t *testing.T) {
	var p Player
	if p.Enqueue() != nil || p.Active() {
		t.Fatal("enqueueing nothing must not start the player")
	}
}

func TestAdvanceUntilEmpty(t *testing.T) {
	var p Player
	p.Enqueue(tracks("a", "b", "c")...)
	var played []string
	for c := p.Current(); c != nil; c = p.Advance() {
		played = append(played, c.Title)
	}
	if !slices.Equal(played, []string{"a", "b", "c"}) {
		t.Fatalf("played = %v, want [a b c]", played)
	}
	if p.Active() {
		t.Fatal("player must be idle after the queue ends")
	}
	if p.Advance() != nil {
		t.Fatal("Advance() on an empty queue must return nil")
	}
}

func TestSnapshot(t *testing.T) {
	var p Player
	empty := p.Snapshot(5)
	if empty.Current != nil || empty.Total != 0 || len(empty.Upcoming) != 0 {
		t.Fatalf("unexpected snapshot of idle player: %+v", empty)
	}
	p.Enqueue(tracks("a", "b", "c", "d")...)
	s := p.Snapshot(2)
	if s.Current.Title != "a" || s.Total != 3 || !slices.Equal(titles(s.Upcoming), []string{"b", "c"}) {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
	if all := p.Snapshot(-1); len(all.Upcoming) != 3 {
		t.Fatalf("negative limit must return the whole queue, got %d", len(all.Upcoming))
	}
	if none := p.Snapshot(0); len(none.Upcoming) != 0 || none.Total != 3 {
		t.Fatalf("zero limit must return no tracks but keep the total, got %+v", none)
	}
	p.Enqueue(Track{Title: "live", IsStream: true, Duration: time.Hour}, Track{Title: "e", Duration: time.Minute})
	if length := p.Snapshot(0).Length; length != time.Minute {
		t.Fatalf("length must sum the whole queue without streams, got %v", length)
	}
}

func TestSnapshotIsIsolated(t *testing.T) {
	var p Player
	p.Enqueue(tracks("a", "b")...)
	s := p.Snapshot(-1)
	s.Upcoming[0].Title = "mutated"
	s.Current.Title = "mutated"
	again := p.Snapshot(-1)
	if again.Upcoming[0].Title != "b" || again.Current.Title != "a" {
		t.Fatal("Snapshot() must return copies")
	}
	c := p.Current()
	c.Title = "mutated"
	if p.Current().Title != "a" {
		t.Fatal("Current() must return a copy")
	}
}

func TestReset(t *testing.T) {
	var p Player
	p.Enqueue(tracks("a", "b")...)
	p.Reset()
	if p.Active() {
		t.Fatal("player must be idle after Reset()")
	}
	if total := p.Snapshot(-1).Total; total != 0 {
		t.Fatalf("queue must be empty after Reset(), got %d", total)
	}
	if start := p.Enqueue(tracks("c")...); start == nil || start.Title != "c" {
		t.Fatal("player must restart after Reset()")
	}
}

func TestPlayerConcurrentEnqueueStartsOnce(t *testing.T) {
	var p Player
	var wg sync.WaitGroup
	var mu sync.Mutex
	starts := 0
	for i := range 100 {
		wg.Go(func() {
			if p.Enqueue(Track{Title: fmt.Sprint(i)}) != nil {
				mu.Lock()
				starts++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if starts != 1 {
		t.Fatalf("player started %d times, want 1", starts)
	}
	if total := p.Snapshot(-1).Total; total != 99 {
		t.Fatalf("queue length = %d, want 99", total)
	}
}

func TestManager(t *testing.T) {
	m := NewManager()
	a := m.Get("1")
	if m.Get("1") != a {
		t.Fatal("Get() must return the same player for a guild")
	}
	m.Get("2").Enqueue(tracks("x")...)
	if active := m.Active(); !slices.Equal(active, []string{"2"}) {
		t.Fatalf("Active() = %v, want [2]", active)
	}
	m.Delete("1")
	if m.Get("1") == a {
		t.Fatal("Delete() must drop the player")
	}
}

func TestFromLavalink(t *testing.T) {
	uri := "https://www.youtube.com/watch?v=id"
	art := "https://i.ytimg.com/vi/id/mqdefault.jpg"
	src := lavalink.Track{
		Encoded: "QAAA",
		Info: lavalink.TrackInfo{
			Identifier: "id",
			Author:     "Artist",
			Title:      "Song",
			Length:     lavalink.Duration(215_000),
			SourceName: "youtube",
			URI:        &uri,
			ArtworkURL: &art,
		},
	}
	got := FromLavalink(src, Requester{ID: "1", Name: "alice"})
	want := Track{
		Encoded: "QAAA", Identifier: "id", Title: "Song", Author: "Artist", URI: uri, ArtworkURL: art,
		Source: "youtube", Duration: 3*time.Minute + 35*time.Second, Requester: "alice", RequesterID: "1",
	}
	if got != want {
		t.Fatalf("FromLavalink() = %+v, want %+v", got, want)
	}
	noLinks := FromLavalink(lavalink.Track{Info: lavalink.TrackInfo{IsStream: true}}, Requester{Name: "bob"})
	if noLinks.URI != "" || noLinks.ArtworkURL != "" || !noLinks.IsStream {
		t.Fatalf("unexpected conversion of track without links: %+v", noLinks)
	}
	if list := FromLavalinkList([]lavalink.Track{src, src}, Requester{Name: "carol"}); len(list) != 2 || list[1].Requester != "carol" {
		t.Fatalf("unexpected list conversion: %+v", list)
	}
}
