package bot

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"discord-sound-bot/config"
	"discord-sound-bot/sound"
	"discord-sound-bot/youtube"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session       *discordgo.Session
	registry      *sound.Registry
	player        *sound.Player
	downloader    *youtube.Downloader
	prefix        string
	soundsDir     string
	stopChannels  map[string]chan struct{}
	stopChannelsMu sync.Mutex
}

func New(cfg *config.Config) (*Bot, error) {
	reg, err := sound.NewRegistry(cfg.SoundsDir + "/registry.json")
	if err != nil {
		return nil, fmt.Errorf("registry: %w", err)
	}

	sounds := reg.List()
	log.Printf("Loaded registry: %d sound(s) registered", len(sounds))

	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("discord: %w", err)
	}

	session.StateEnabled = true
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildVoiceStates | discordgo.IntentGuildMessages

	b := &Bot{
		session:      session,
		registry:     reg,
		player:       sound.NewPlayer(cfg.SoundsDir),
		downloader:   youtube.NewDownloader(cfg.SoundsDir),
		prefix:       cfg.CommandPrefix,
		soundsDir:    cfg.SoundsDir,
		stopChannels: make(map[string]chan struct{}),
	}

	session.AddHandler(b.handleMessage)
	session.AddHandler(b.handleReady)

	return b, nil
}

func (b *Bot) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("Connected to Discord as %s (guilds: %d)", r.User.Username, len(r.Guilds))
}

func (b *Bot) Start() error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("open: %w", err)
	}
	return nil
}

func (b *Bot) Close() {
	log.Println("Closing Discord session...")
	b.session.Close()
}

func (b *Bot) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !strings.HasPrefix(m.Content, b.prefix) {
		return
	}

	content := strings.TrimPrefix(m.Content, b.prefix)
	parts := strings.Fields(content)
	if len(parts) == 0 {
		return
	}

	command := strings.ToLower(parts[0])
	args := parts[1:]

	log.Printf("[CMD] user=%s guild=%s channel=%s command=%q args=%v",
		m.Author.Username, m.GuildID, m.ChannelID, command, args)

	switch command {
	case "add":
		b.handleAdd(s, m, args)
	case "random":
		b.playRandom(s, m)
	case "list":
		b.handleList(s, m)
	case "stop":
		b.handleStop(s, m)
	default:
		b.handleDynamicPlay(s, m, command)
	}
}

func (b *Bot) handleAdd(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	if len(args) < 2 {
		s.ChannelMessageSend(m.ChannelID, "Usage: `!add <sound_name> <video_url> [start] [end]`")
		return
	}

	name := strings.ToLower(args[0])
	url := args[1]
	startStr := ""
	endStr := ""
	if len(args) >= 3 {
		startStr = args[2]
	}
	if len(args) >= 4 {
		endStr = args[3]
	}

	log.Printf("[ADD] name=%q url=%q start=%q end=%q", name, url, startStr, endStr)

	existingDCA := b.soundsDir + "/" + name + ".dca"
	if _, exists := b.registry.Get(name); exists {
		log.Printf("[ADD] sound %q exists, will overwrite", name)
		os.Remove(existingDCA)
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Downloading %q...", name))

	audioFile, err := b.downloader.Download(name, url)
	if err != nil {
		log.Printf("[ADD] download failed for %q: %v", name, err)
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Download failed: %v", err))
		return
	}
	log.Printf("[ADD] downloaded %q -> %s", name, audioFile)

	dcaFilePath := b.soundsDir + "/" + name + ".dca"

	startSec := sound.ParseTime(startStr)
	endSec := sound.ParseTime(endStr)

	opts := &sound.EncodeOptions{
		StartTime: startSec,
		EndTime:   endSec,
		Bitrate:   96,
	}

	if startStr != "" && endStr != "" {
		log.Printf("[ADD] trim: start=%ds end=%ds", startSec, endSec)
	} else if startStr != "" {
		log.Printf("[ADD] trim: start=%ds", startSec)
	} else if endStr != "" {
		log.Printf("[ADD] trim: end=%ds", endSec)
	}

	log.Printf("[ADD] encoding %q to DCA...", name)
	if err := sound.EncodeToDCA(audioFile, dcaFilePath, opts); err != nil {
		log.Printf("[ADD] encoding failed for %q: %v", name, err)
		os.Remove(audioFile)
		os.Remove(dcaFilePath)
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Encoding failed: %v", err))
		return
	}

	os.Remove(audioFile)
	log.Printf("[ADD] DCA written to %s", dcaFilePath)

	entry := &sound.SoundEntry{
		Name:    name,
		URL:     url,
		File:    name + ".dca",
		Start:   startStr,
		End:     endStr,
		AddedAt: time.Now().Format(time.RFC3339),
	}

	if err := b.registry.Add(entry); err != nil {
		log.Printf("[ADD] registry add failed for %q: %v", name, err)
		os.Remove(dcaFilePath)
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Registration failed: %v", err))
		return
	}

	log.Printf("[ADD] sound %q added successfully", name)
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sound %q added!", name))
}

func (b *Bot) handleDynamicPlay(s *discordgo.Session, m *discordgo.MessageCreate, name string) {
	if _, ok := b.registry.Get(name); !ok {
		log.Printf("[PLAY] sound %q not found, ignoring", name)
		return
	}
	log.Printf("[PLAY] dynamic command matched sound %q", name)
	b.play(s, m, name)
}

func (b *Bot) play(s *discordgo.Session, m *discordgo.MessageCreate, name string) {
	entry, ok := b.registry.Get(name)
	if !ok {
		log.Printf("[PLAY] sound %q not found in registry", name)
		return
	}

	log.Printf("[PLAY] user=%s looking up voice channel for guild=%s", m.Author.Username, m.GuildID)
	vcID, err := findUserVoiceChannel(s, m.GuildID, m.Author.ID)
	if err != nil {
		log.Printf("[PLAY] user %s is not in a voice channel: %v", m.Author.Username, err)
		s.ChannelMessageSend(m.ChannelID, "Join a voice channel first.")
		return
	}
	log.Printf("[PLAY] user=%s voice_channel=%s, playing %q", m.Author.Username, vcID, name)

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Playing %q", name))

	stop := make(chan struct{})
	b.stopChannelsMu.Lock()
	b.stopChannels[m.GuildID] = stop
	b.stopChannelsMu.Unlock()

	err = b.player.Play(s, m.GuildID, vcID, entry.File, stop)

	b.stopChannelsMu.Lock()
	delete(b.stopChannels, m.GuildID)
	b.stopChannelsMu.Unlock()

	if err != nil {
		log.Printf("[PLAY] playback failed for %q: %v", name, err)
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Playback failed: %v", err))
		return
	}

	log.Printf("[PLAY] finished playing %q", name)
}

func (b *Bot) playRandom(s *discordgo.Session, m *discordgo.MessageCreate) {
	entry := b.registry.Random()
	if entry == nil {
		log.Printf("[RANDOM] no sounds registered")
		s.ChannelMessageSend(m.ChannelID, "No sounds registered. Add some with `!add`.")
		return
	}
	log.Printf("[RANDOM] selected sound %q", entry.Name)

	vcID, err := findUserVoiceChannel(s, m.GuildID, m.Author.ID)
	if err != nil {
		log.Printf("[RANDOM] user %s not in voice channel: %v", m.Author.Username, err)
		s.ChannelMessageSend(m.ChannelID, "Join a voice channel first.")
		return
	}
	log.Printf("[RANDOM] user=%s voice_channel=%s", m.Author.Username, vcID)

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Random: %q", entry.Name))

	stop := make(chan struct{})
	b.stopChannelsMu.Lock()
	b.stopChannels[m.GuildID] = stop
	b.stopChannelsMu.Unlock()

	err = b.player.Play(s, m.GuildID, vcID, entry.File, stop)

	b.stopChannelsMu.Lock()
	delete(b.stopChannels, m.GuildID)
	b.stopChannelsMu.Unlock()

	if err != nil {
		log.Printf("[RANDOM] playback failed: %v", err)
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Playback failed: %v", err))
		return
	}

	log.Printf("[RANDOM] finished playing %q", entry.Name)
}

func (b *Bot) handleStop(s *discordgo.Session, m *discordgo.MessageCreate) {
	b.stopChannelsMu.Lock()
	stop, ok := b.stopChannels[m.GuildID]
	b.stopChannelsMu.Unlock()

	if !ok {
		s.ChannelMessageSend(m.ChannelID, "No sound is currently playing.")
		return
	}

	log.Printf("[STOP] user=%s stopping playback in guild %s", m.Author.Username, m.GuildID)
	close(stop)
	s.ChannelMessageSend(m.ChannelID, "Stopped.")
}

func (b *Bot) handleList(s *discordgo.Session, m *discordgo.MessageCreate) {
	sounds := b.registry.List()

	log.Printf("[LIST] user=%s requested list: %d sound(s) found", m.Author.Username, len(sounds))

	if len(sounds) == 0 {
		s.ChannelMessageSend(m.ChannelID, "No sounds registered. Add some with `!add`.")
		return
	}

	var sb strings.Builder
	sb.WriteString("**Registered Sounds:**\n")
	for _, snd := range sounds {
		sb.WriteString(fmt.Sprintf("• `%s`", snd.Name))
		if snd.Start != "" || snd.End != "" {
			sb.WriteString(fmt.Sprintf(" [%s - %s]", snd.Start, snd.End))
		}
		sb.WriteString("\n")
	}

	s.ChannelMessageSend(m.ChannelID, sb.String())
}

func findUserVoiceChannel(s *discordgo.Session, guildID, userID string) (string, error) {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		return "", fmt.Errorf("guild lookup: %w", err)
	}

	log.Printf("[VOICE] scanning %d voice states in guild %s for user %s", len(guild.VoiceStates), guildID, userID)
	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			log.Printf("[VOICE] found user %s in channel %s", userID, vs.ChannelID)
			return vs.ChannelID, nil
		}
	}
	return "", fmt.Errorf("user not in a voice channel")
}
