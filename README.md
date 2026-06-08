# Discord Sound Bot

A Discord bot written in Go that registers sounds from YouTube videos and plays them in voice channels.

## Commands

| Command | Description |
|---|---|
| `!add <name> <url> [start] [end]` | Register a sound from a YouTube video, optionally trimmed to `[start, end]` (seconds or `MM:SS`). Overwrites if name already exists. |
| `!<name>` | Play a registered sound by name |
| `!random` | Play a random registered sound |
| `!list` | List all registered sounds |
| `!stop` | Stop the currently playing sound and disconnect |

## How It Works

### Adding a Sound (`!add`)

1. `yt-dlp` downloads the best audio stream from the YouTube URL
2. `ffmpeg` re-encodes the audio to Opus in an OGG container (with optional `-ss`/`-t` trimming)
3. The bot reads the OGG stream, extracts Opus packets (skipping the 2 header packets), and writes them in DCA0 format (2-byte LE length prefix + Opus frame)
4. The `.dca` file is saved to the configured sounds directory
5. An entry is added to `registry.json` inside the same directory

### Auto-Registration at Startup

On every startup, the bot scans `sounds/` for audio files not yet in the registry:

- `*.dca` files are registered directly
- Common audio formats (`mp3`, `wav`, `ogg`, `m4a`, `webm`, `flac`, `opus`, `aac`, `wma`) are converted to `.dca` via ffmpeg, the original file is deleted, and the `.dca` is registered

This means you can simply drop audio files into the `sounds/` directory and restart the bot — they'll be available as `!<name>` without needing `!add`.

Files are skipped if a `.dca` with the same name already exists, if the name is already registered, or if the name starts with `.` (hidden files).

### Playing a Sound (`!<name>`, `!random`)

1. The bot looks up the caller's current voice channel via Discord's voice states
2. Joins the voice channel using discordgo (with DAVE/E2EE protocol support)
3. Opens the `.dca` file and reads Opus frames via the `dca` decoder
4. Sends each Opus frame to the voice connection's `OpusSend` channel
5. Disconnects automatically when playback completes or stops via `!stop`

### Architecture

```
main.go              → Entry point, config loading, signal handling
config/config.go     → Env-based configuration
bot/bot.go           → Discord session, message routing, command handlers
sound/registry.go    → JSON file registry (mutex-guarded CRUD), auto-registration
sound/player.go      → Voice channel join, DCA streaming, playback
sound/encoder.go     → Custom DCA encoder (ffmpeg → Ogg Opus → DCA0)
youtube/downloader.go → yt-dlp subprocess wrapper
```

- **No CGO** — the `dca` package and the custom encoder use `os/exec` to call `ffmpeg` for audio processing; no Go-based Opus library is needed
- **DAVE/E2EE** — uses `yeongaori/discordgo` fork with DAVE protocol support (required by Discord since 2025)
- **Registry** — `registry.json` is stored inside the mounted `sounds/` volume, persisting across restarts

## Setup

### Prerequisites

- Docker and Docker Compose
- A Discord bot token from [Discord Developer Portal](https://discord.com/developers/applications)

### Discord Bot Configuration

1. Create a **New Application** at the Developer Portal
2. Go to **Bot** → **Reset Token** and copy it
3. Enable **Message Content Intent** (required to read `!` commands)
4. Go to **OAuth2 > URL Generator**:
   - **Scopes**: `bot`
   - **Bot Permissions**: `Send Messages`, `Connect`, `Speak`
5. Use the generated URL to invite the bot to your server

The bot must have **Connect** and **Speak** permissions on the voice channels you want to use.

### Running

```bash
# Create an env file with your token
echo "DISCORD_TOKEN=your_token_here" > .env

# Start
docker compose up -d

# Watch logs
docker compose logs -f
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DISCORD_TOKEN` | (required) | Discord bot token |
| `SOUNDS_DIR` | `./sounds` | Directory for audio files and registry |
| `COMMAND_PREFIX` | `!` | Command prefix |

### Volumes

`./sounds` on the host is mounted to `/app/sounds` in the container, persisting both `.dca` files and `registry.json`.

### Network

`network_mode: host` is used because Discord voice streams audio over UDP, which doesn't work through Docker's default NAT bridge network.
