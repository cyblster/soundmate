package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/snowflake/v2"

	"github.com/cyblster/soundmate/internal/config"
	"github.com/cyblster/soundmate/internal/cooldown"
	"github.com/cyblster/soundmate/internal/debounce"
	"github.com/cyblster/soundmate/internal/guilds"
	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/logger"
	"github.com/cyblster/soundmate/internal/music"
	"github.com/cyblster/soundmate/internal/search"
	"github.com/cyblster/soundmate/internal/storage"
	"github.com/cyblster/soundmate/internal/vote"
)

const (
	renderDelay     = 750 * time.Millisecond
	searchTTL       = 60 * time.Second
	idleTimeout     = 30 * time.Second
	requestTimeout  = 15 * time.Second
	shutdownTimeout = 10 * time.Second
	defaultSlowMode = time.Minute
	nodeName        = "main"
)

type Store interface {
	storage.GuildRepository
	storage.HistoryRepository
}

type pendingSearch struct {
	interaction *discordgo.Interaction
	tracks      []music.Track
}

type Bot struct {
	cfg       config.Config
	log       *slog.Logger
	session   *discordgo.Session
	link      disgolink.Client
	store     Store
	guilds    *guilds.Registry
	players   *music.Manager
	renderer  *debounce.Coalescer
	searches  *search.Store[pendingSearch]
	votes     *vote.Box
	cooldowns *cooldown.Tracker
	rendered  sync.Map
	voiceJoin func(guildID, channelID string, mute, deaf bool) error
	voiceMu   sync.Mutex

	idleMu      sync.Mutex
	idleTimers  map[string]*time.Timer
	idleTimeout time.Duration

	nodeReady atomic.Bool
	readyOnce sync.Once
}

func New(cfg config.Config, log *slog.Logger, store Store) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	discordgo.Logger = logger.DiscordAdapter(log)
	session.LogLevel = logger.DiscordLevel(cfg.Debug)
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildVoiceStates
	session.StateEnabled = true

	b := &Bot{
		cfg:         cfg,
		log:         log,
		session:     session,
		store:       store,
		guilds:      guilds.NewRegistry(store),
		players:     music.NewManager(),
		votes:       vote.NewBox(),
		cooldowns:   cooldown.New(),
		idleTimers:  map[string]*time.Timer{},
		idleTimeout: idleTimeout,
	}
	b.voiceJoin = session.ChannelVoiceJoinManual
	b.renderer = debounce.New(renderDelay, b.renderLogged)
	b.searches = search.NewStore(searchTTL, func(p pendingSearch) {
		_ = b.session.InteractionResponseDelete(p.interaction)
	})
	return b, nil
}

func (b *Bot) Run(ctx context.Context) error {
	count, err := b.guilds.Load(ctx)
	if err != nil {
		return fmt.Errorf("load guilds: %w", err)
	}
	b.log.Info("guild configurations loaded", "count", count)

	me, err := b.session.User("@me")
	if err != nil {
		var restErr *discordgo.RESTError
		if errors.As(err, &restErr) && restErr.Response != nil && restErr.Response.StatusCode == 401 {
			return errors.New("discord rejected BOT_TOKEN, check the token in your environment")
		}
		return fmt.Errorf("fetch bot user: %w", err)
	}
	userID, err := snowflake.Parse(me.ID)
	if err != nil {
		return fmt.Errorf("parse bot user id: %w", err)
	}
	b.link = disgolink.New(userID,
		disgolink.WithLogger(logger.LavalinkAdapter(b.log)),
		disgolink.WithListenerFunc(b.onTrackStart),
		disgolink.WithListenerFunc(b.onTrackEnd),
		disgolink.WithListenerFunc(b.onTrackException),
		disgolink.WithListenerFunc(b.onTrackStuck),
		disgolink.WithListenerFunc(b.onWebSocketClosed),
	)

	b.session.AddHandler(b.onReady)
	b.session.AddHandler(b.onInteraction)
	b.session.AddHandler(b.onVoiceStateUpdate)
	b.session.AddHandler(b.onVoiceServerUpdate)
	b.session.AddHandler(b.onGuildDelete)

	b.log.Info("connecting to discord", "user", me.Username)
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("open discord gateway: %w", err)
	}

	go b.connectLavalink(ctx)

	<-ctx.Done()
	b.shutdown()
	return nil
}

func (b *Bot) connectLavalink(ctx context.Context) {
	address := b.cfg.Lavalink.Address()
	b.log.Info("connecting to lavalink", "address", address)
	node, err := b.link.AddNode(ctx, disgolink.NodeConfig{
		Name:     nodeName,
		Address:  address,
		Password: b.cfg.Lavalink.Password,
	})
	if err != nil {
		if ctx.Err() == nil {
			b.log.Error("failed to connect to lavalink", "error", err)
		}
		return
	}
	versionCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	version, err := node.Version(versionCtx)
	if err != nil {
		version = "unknown"
	}
	b.nodeReady.Store(true)
	b.log.Info("lavalink node connected", "address", address, "version", version)
	go b.watchFrames(ctx, node)
}

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	b.log.Info("connected to discord", "user", r.User.Username, "guilds", len(r.Guilds), "session", r.SessionID)
	appID := r.User.ID
	if r.Application != nil && r.Application.ID != "" {
		appID = r.Application.ID
	}
	b.readyOnce.Do(func() {
		go b.initialize(appID)
	})
}

func (b *Bot) initialize(appID string) {
	if _, err := b.session.ApplicationCommandBulkOverwrite(appID, "", Commands()); err != nil {
		b.log.Error("failed to register slash commands", "error", err)
	} else {
		b.log.Info("slash commands registered", "count", len(Commands()))
	}
	restored := 0
	for _, g := range b.guilds.All() {
		if err := b.render(g.GuildID); err != nil {
			b.log.Warn("failed to restore player", "guild", b.guildName(g.GuildID), "error", err)
			continue
		}
		restored++
	}
	b.log.Info("ready to play", "servers", restored)
}

func (b *Bot) onGuildDelete(_ *discordgo.Session, e *discordgo.GuildDelete) {
	if e.Unavailable {
		b.log.Warn("guild became unavailable", "guild_id", e.ID)
		return
	}
	b.cancelIdleLeave(e.ID)
	b.destroyPlayer(e.ID)
	b.players.Delete(e.ID)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := b.guilds.Delete(ctx, e.ID); err != nil {
		b.log.Error("failed to delete guild configuration", "guild_id", e.ID, "error", err)
	}
	name := e.ID
	if e.BeforeDelete != nil {
		name = e.BeforeDelete.Name
	}
	b.log.Info("removed from server", "guild", name)
}

func (b *Bot) shutdown() {
	b.log.Info("shutting down")
	b.renderer.Close()
	active := b.players.Active()
	for _, guildID := range b.idleGuilds() {
		if !slices.Contains(active, guildID) {
			active = append(active, guildID)
		}
	}
	var wg sync.WaitGroup
	for _, guildID := range active {
		wg.Go(func() {
			b.leave(guildID)
			if err := b.render(guildID); err != nil {
				b.log.Warn("failed to reset player message", "guild", b.guildName(guildID), "error", err)
			}
		})
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		b.log.Info("active players stopped", "count", len(active))
	case <-time.After(shutdownTimeout):
		b.log.Warn("timed out while stopping active players", "count", len(active))
	}
	if b.link != nil {
		b.link.Close()
	}
	if err := b.session.Close(); err != nil {
		b.log.Warn("failed to close discord session", "error", err)
	}
	b.log.Info("shutdown complete")
}

func (b *Bot) guildName(guildID string) string {
	if g, err := b.session.State.Guild(guildID); err == nil && g.Name != "" {
		return g.Name
	}
	return guildID
}

func (b *Bot) guildLocale(guildID string) i18n.Locale {
	if g, ok := b.guilds.Get(guildID); ok && g.Locale != "" {
		return g.Locale
	}
	return i18n.Default
}

func (b *Bot) messagesFor(i *discordgo.Interaction) *i18n.Messages {
	if l, ok := i18n.FromDiscord(i.Locale); ok {
		return i18n.Get(l)
	}
	return i18n.Get(i18n.Default)
}

func serverLocale(i *discordgo.Interaction) i18n.Locale {
	if i.GuildLocale != nil {
		if l, ok := i18n.FromDiscord(*i.GuildLocale); ok {
			return l
		}
	}
	return i18n.Default
}

func (b *Bot) timeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), requestTimeout)
}

func toSnowflake(id string) snowflake.ID {
	v, _ := snowflake.Parse(id)
	return v
}
