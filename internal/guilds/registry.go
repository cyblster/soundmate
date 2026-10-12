package guilds

import (
	"context"
	"sync"

	"github.com/cyblster/soundmate/internal/storage"
)

type Registry struct {
	repo   storage.GuildRepository
	mu     sync.RWMutex
	guilds map[string]storage.Guild
}

func NewRegistry(repo storage.GuildRepository) *Registry {
	return &Registry{repo: repo, guilds: map[string]storage.Guild{}}
}

func (r *Registry) Load(ctx context.Context) (int, error) {
	list, err := r.repo.ListGuilds(ctx)
	if err != nil {
		return 0, err
	}
	loaded := make(map[string]storage.Guild, len(list))
	for _, g := range list {
		loaded[g.GuildID] = g
	}
	r.mu.Lock()
	r.guilds = loaded
	r.mu.Unlock()
	return len(loaded), nil
}

func (r *Registry) Get(guildID string) (storage.Guild, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.guilds[guildID]
	return g, ok
}

func (r *Registry) All() []storage.Guild {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]storage.Guild, 0, len(r.guilds))
	for _, g := range r.guilds {
		result = append(result, g)
	}
	return result
}

func (r *Registry) Save(ctx context.Context, g storage.Guild) error {
	if err := r.repo.UpsertGuild(ctx, g); err != nil {
		return err
	}
	r.mu.Lock()
	r.guilds[g.GuildID] = g
	r.mu.Unlock()
	return nil
}

func (r *Registry) Delete(ctx context.Context, guildID string) error {
	r.mu.Lock()
	delete(r.guilds, guildID)
	r.mu.Unlock()
	return r.repo.DeleteGuild(ctx, guildID)
}
