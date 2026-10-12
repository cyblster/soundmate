package guilds

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/storage"
)

type fakeRepo struct {
	guilds    map[string]storage.Guild
	listCalls int
	failWith  error
}

func newFakeRepo(guilds ...storage.Guild) *fakeRepo {
	r := &fakeRepo{guilds: map[string]storage.Guild{}}
	for _, g := range guilds {
		r.guilds[g.GuildID] = g
	}
	return r
}

func (r *fakeRepo) UpsertGuild(_ context.Context, g storage.Guild) error {
	if r.failWith != nil {
		return r.failWith
	}
	r.guilds[g.GuildID] = g
	return nil
}

func (r *fakeRepo) ListGuilds(context.Context) ([]storage.Guild, error) {
	r.listCalls++
	if r.failWith != nil {
		return nil, r.failWith
	}
	var result []storage.Guild
	for _, g := range r.guilds {
		result = append(result, g)
	}
	return result, nil
}

func (r *fakeRepo) DeleteGuild(_ context.Context, id string) error {
	delete(r.guilds, id)
	return r.failWith
}

func TestLoadAndGetServeFromMemory(t *testing.T) {
	repo := newFakeRepo(
		storage.Guild{GuildID: "1", ChannelID: "10"},
		storage.Guild{GuildID: "2", ChannelID: "20", Locale: i18n.Russian},
	)
	r := NewRegistry(repo)
	n, err := r.Load(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("Load() = %d, %v", n, err)
	}
	for range 10 {
		if g, ok := r.Get("2"); !ok || g.Locale != i18n.Russian {
			t.Fatalf("Get(2) = %+v, %v", g, ok)
		}
	}
	if repo.listCalls != 1 {
		t.Fatalf("repository was queried %d times, want 1", repo.listCalls)
	}
	if _, ok := r.Get("3"); ok {
		t.Fatal("unknown guild must not be found")
	}
	ids := []string{}
	for _, g := range r.All() {
		ids = append(ids, g.GuildID)
	}
	slices.Sort(ids)
	if strings.Join(ids, ",") != "1,2" {
		t.Fatalf("All() = %v", ids)
	}
}

func TestSaveUpdatesCacheOnlyOnSuccess(t *testing.T) {
	repo := newFakeRepo()
	r := NewRegistry(repo)
	g := storage.Guild{GuildID: "1", ChannelID: "10"}
	if err := r.Save(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Get("1"); !ok || got != g || repo.guilds["1"] != g {
		t.Fatal("Save() must persist and cache the guild")
	}
	repo.failWith = errors.New("db down")
	if err := r.Save(context.Background(), storage.Guild{GuildID: "1", ChannelID: "99"}); err == nil {
		t.Fatal("Save() must return repository errors")
	}
	if got, _ := r.Get("1"); got.ChannelID != "10" {
		t.Fatal("cache must not change when the repository fails")
	}
}

func TestDelete(t *testing.T) {
	repo := newFakeRepo(storage.Guild{GuildID: "1"})
	r := NewRegistry(repo)
	if _, err := r.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("1"); ok || len(repo.guilds) != 0 {
		t.Fatal("Delete() must remove the guild from cache and repository")
	}
}

func TestLoadFailureKeepsCache(t *testing.T) {
	repo := newFakeRepo(storage.Guild{GuildID: "1"})
	r := NewRegistry(repo)
	if _, err := r.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	repo.failWith = errors.New("db down")
	if _, err := r.Load(context.Background()); err == nil {
		t.Fatal("Load() must return repository errors")
	}
	if _, ok := r.Get("1"); !ok {
		t.Fatal("failed reload must keep the previous cache")
	}
}
