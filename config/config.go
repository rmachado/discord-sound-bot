package config

import (
	"os"
)

type Config struct {
	DiscordToken  string
	SoundsDir     string
	CommandPrefix string
}

func Load() *Config {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		panic("DISCORD_TOKEN environment variable is required")
	}
	soundsDir := os.Getenv("SOUNDS_DIR")
	if soundsDir == "" {
		soundsDir = "./sounds"
	}
	prefix := os.Getenv("COMMAND_PREFIX")
	if prefix == "" {
		prefix = "!"
	}
	return &Config{
		DiscordToken:  token,
		SoundsDir:     soundsDir,
		CommandPrefix: prefix,
	}
}
