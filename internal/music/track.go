package music

import (
	"time"

	"github.com/disgoorg/disgolink/v3/lavalink"
)

type Requester struct {
	ID   string
	Name string
}

type Track struct {
	Encoded     string
	Identifier  string
	Title       string
	Author      string
	URI         string
	ArtworkURL  string
	Source      string
	Duration    time.Duration
	IsStream    bool
	Requester   string
	RequesterID string
}

func FromLavalink(t lavalink.Track, requester Requester) Track {
	track := Track{
		Encoded:     t.Encoded,
		Identifier:  t.Info.Identifier,
		Title:       t.Info.Title,
		Author:      t.Info.Author,
		Source:      t.Info.SourceName,
		Duration:    time.Duration(t.Info.Length.Milliseconds()) * time.Millisecond,
		IsStream:    t.Info.IsStream,
		Requester:   requester.Name,
		RequesterID: requester.ID,
	}
	if t.Info.URI != nil {
		track.URI = *t.Info.URI
	}
	if t.Info.ArtworkURL != nil {
		track.ArtworkURL = *t.Info.ArtworkURL
	}
	return track
}

func FromLavalinkList(tracks []lavalink.Track, requester Requester) []Track {
	result := make([]Track, len(tracks))
	for i, t := range tracks {
		result[i] = FromLavalink(t, requester)
	}
	return result
}
