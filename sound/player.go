package sound

import (
	"log"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)

type Player struct {
	soundsDir string
}

func NewPlayer(soundsDir string) *Player {
	return &Player{soundsDir: soundsDir}
}

func (p *Player) Play(session *discordgo.Session, guildID, channelID string, fileName string, stop <-chan struct{}) error {
	filePath := p.soundsDir + "/" + fileName

	log.Printf("[PLAYER] joining voice channel %s in guild %s", channelID, guildID)

	vc, err := session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		log.Printf("[PLAYER] failed to join voice channel: %v", err)
		return err
	}

	defer func() {
		log.Printf("[PLAYER] disconnecting from voice channel %s", channelID)
		vc.Disconnect()
	}()

	log.Printf("[PLAYER] joined voice channel %s, starting playback", channelID)
	vc.Speaking(true)
	defer vc.Speaking(false)

	log.Printf("[PLAYER] opening DCA file %s", filePath)
	f, err := os.Open(filePath)
	if err != nil {
		log.Printf("[PLAYER] failed to open DCA file %s: %v", filePath, err)
		return err
	}
	defer f.Close()

	decoder := dca.NewDecoder(f)

	frameCount := 0
	for {
		select {
		case <-stop:
			log.Printf("[PLAYER] playback stopped by !stop after %d frames", frameCount)
			return nil
		default:
		}

		frame, err := decoder.OpusFrame()
		if err != nil {
			log.Printf("[PLAYER] end of stream after %d frames: %v", frameCount, err)
			break
		}

		select {
		case vc.OpusSend <- frame:
			frameCount++
		case <-time.After(time.Second):
			log.Printf("[PLAYER] timeout sending opus frame %d", frameCount)
			return nil
		case <-stop:
			log.Printf("[PLAYER] playback stopped by !stop after %d frames", frameCount)
			return nil
		}
	}

	log.Printf("[PLAYER] playback complete: %d frames sent", frameCount)
	return nil
}
