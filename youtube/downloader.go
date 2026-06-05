package youtube

import (
	"fmt"
	"log"
	"os/exec"
)

type Downloader struct {
	soundsDir string
}

func NewDownloader(soundsDir string) *Downloader {
	return &Downloader{soundsDir: soundsDir}
}

func (d *Downloader) Download(name, url string) (string, error) {
	outputPath := d.soundsDir + "/" + name + ".%(ext)s"

	log.Printf("[YT-DLP] downloading %q from %s", name, url)

	args := []string{
		"-f", "bestaudio",
		"--extract-audio",
		"--audio-format", "opus",
		"-o", outputPath,
		"--no-playlist",
		"--print", "after_move:filepath",
		url,
	}

	log.Printf("[YT-DLP] args: %v", args)

	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			log.Printf("[YT-DLP] download failed for %q: %s", name, stderr)
			return "", fmt.Errorf("yt-dlp error: %s", stderr)
		}
		log.Printf("[YT-DLP] download failed for %q: %v", name, err)
		return "", fmt.Errorf("yt-dlp: %w", err)
	}

	actualPath := string(out)
	if len(actualPath) > 0 && actualPath[len(actualPath)-1] == '\n' {
		actualPath = actualPath[:len(actualPath)-1]
	}
	if actualPath == "" {
		actualPath = d.soundsDir + "/" + name + ".opus"
	}

	log.Printf("[YT-DLP] downloaded %q to %s", name, actualPath)
	return actualPath, nil
}
