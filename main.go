package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"discord-sound-bot/bot"
	"discord-sound-bot/config"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	cfg := config.Load()

	log.Printf("Starting bot (sounds_dir=%s, prefix=%q)", cfg.SoundsDir, cfg.CommandPrefix)

	b, err := bot.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	if err := b.Start(); err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	log.Println("Bot is running. Press Ctrl-C to stop.")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc

	log.Println("Shutting down...")
	b.Close()
	log.Println("Bot stopped.")
}
