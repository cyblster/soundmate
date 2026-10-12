# soundmate

**English** | [Русский](README.ru.md)

soundmate is a self-hosted music bot for Discord. It plays audio from YouTube and SoundCloud in voice
channels and is controlled through a persistent player in a dedicated text channel, with slash
commands available as an alternative.

## Features

- Persistent player and queue messages, updated in place
- YouTube or SoundCloud search with a selection of the top five results, and support for direct
  links to tracks and playlists
- Playback of YouTube live streams
- Per-server playback history of the last 20 tracks
- English and Russian interface, configurable per server

## Requirements

- A server with [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- A Discord application with a bot user

## Installation

1. Create an application in the [Discord Developer Portal](https://discord.com/developers/applications),
   add a bot user and copy its token. Privileged gateway intents are not required.
2. Invite the bot to your Discord server. Replace `<APP_ID>` with the ID of your application:

   ```
   https://discord.com/oauth2/authorize?client_id=<APP_ID>&scope=bot+applications.commands&permissions=271666192
   ```

3. Create the configuration file and set `BOT_TOKEN` in it:

   ```bash
   cp .env.sample .env
   ```

4. Build and start the services:

   ```bash
   docker compose up -d --build
   ```

5. In Discord, open the text channel intended for music and run `/setup`. To use the Russian
   interface, run `/setup language:ru`.

## Usage

| Control                       | Available to                       | Action                                                    |
|-------------------------------|------------------------------------|-----------------------------------------------------------|
| `/setup`                      | Administrators                     | Designates the current channel as the music channel. Option `language`: `en` or `ru`, defaults to the server language. Option `slowmode`: seconds between tracks from one member, 0 to 21600, where 0 turns it off. Keeps the current value and defaults to 60 for a new channel |
| 💬, `/add`                     | Members in a voice channel         | Opens a form for a search query or link and the search source. `/add query:<text or link>` searches immediately, `source:SoundCloud` searches SoundCloud instead of YouTube |
| ⏭️, `/skip`                    | Members in the bot's voice channel | Votes to skip the current track. The member who added the track skips it at once |
| ⏹️, `/disconnect`              | Members in the bot's voice channel | Votes to stop playback, clear the queue and leave the voice channel |
| 🔖, `/history`                 | All members                        | Displays the last 20 tracks played on the server          |

A vote passes once more than half of the listeners in the bot's voice channel agree, so a lone
listener acts at once. Pressing the button again withdraws the vote. Votes reset when the track
changes, and votes of members who leave the channel stop counting.

Slow mode lets every member add one track or playlist per chosen period. The player notes when it
is on, and a member who adds too early sees when the next track can be added. Members with the
Manage Messages, Manage Channels or Administrator permission are exempt, as in Discord's own
slow mode.

Slash commands are available in every text channel of the server once `/setup` has been run.
Responses to them are visible only to the member who issued the command. Running `/setup` in a
different channel moves the player to that channel.

## Configuration

The bot reads its configuration from the `.env` file.

| Variable     | Required | Default    | Description                       |
|--------------|:--------:|------------|-----------------------------------|
| `BOT_TOKEN`  | Yes      |            | Discord bot token                 |
| `LL_OPTIONS` | No       | `-Xmx512M` | JVM options for Lavalink          |
| `DEBUG`      | No       | `false`    | Enables debug logging             |

PostgreSQL and Lavalink are accessible only within the internal Docker network. No ports are
published on the host.

## Troubleshooting

| Symptom                               | Resolution                                                       |
|---------------------------------------|------------------------------------------------------------------|
| Tracks are skipped or fail to play    | YouTube changes frequently. Update the YouTube plugin version in `lavalink/application.yaml`, then run `docker compose restart soundmate-ll` |
| The bot does not respond              | Review the logs with `docker compose logs -f soundmate-bot`      |

## License

This project is licensed under the [MIT License](LICENSE). You may use, modify and distribute it freely, provided that the copyright notice crediting the author is retained.
