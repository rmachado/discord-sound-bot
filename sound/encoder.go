package sound

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/jonas747/ogg"
)

type EncodeOptions struct {
	StartTime int
	EndTime   int
	Bitrate   int
}

func EncodeToDCA(inputPath, outputPath string, opts *EncodeOptions) error {
	if opts == nil {
		opts = &EncodeOptions{Bitrate: 96}
	}
	if opts.Bitrate == 0 {
		opts.Bitrate = 96
	}

	args := []string{
		"-y",
		"-stats",
		"-i", inputPath,
		"-map", "0:a",
		"-af", "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true",
		"-acodec", "libopus",
		"-f", "ogg",
		"-ar", "48000",
		"-ac", "2",
		"-b:a", fmt.Sprintf("%dk", opts.Bitrate),
		"-application", "audio",
		"-frame_duration", "20",
		"-vbr", "on",
		"-compression_level", "10",
	}

	if opts.StartTime > 0 {
		args = append(args, "-ss", strconv.Itoa(opts.StartTime))
	}
	if opts.EndTime > 0 && opts.EndTime > opts.StartTime {
		args = append(args, "-t", strconv.Itoa(opts.EndTime-opts.StartTime))
	}

	args = append(args, "pipe:1")

	log.Printf("[ENCODER] ffmpeg args: %v", args)

	cmd := exec.Command("ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				line := strings.TrimRight(string(buf[:n]), "\r\n")
				if line != "" {
					log.Printf("[FFMPEG] %s", line)
				}
			}
			if err != nil {
				return
			}
		}
	}()

	outFile, err := os.Create(outputPath)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return fmt.Errorf("create output: %w", err)
	}
	defer outFile.Close()

	frameCount, err := writeDCAFromOgg(outFile, stdout)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return fmt.Errorf("encode: %w", err)
	}

	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("ffmpeg exit: %w", err)
	}

	log.Printf("[ENCODER] wrote %d frames to %s", frameCount, outputPath)
	return nil
}

func writeDCAFromOgg(out io.Writer, r io.Reader) (int, error) {
	decoder := ogg.NewPacketDecoder(ogg.NewDecoder(r))

	skipPackets := 2
	frameCount := 0

	for {
		packet, _, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			return frameCount, fmt.Errorf("ogg decode: %w", err)
		}

		if skipPackets > 0 {
			skipPackets--
			continue
		}

		if err := binary.Write(out, binary.LittleEndian, int16(len(packet))); err != nil {
			return frameCount, fmt.Errorf("write size: %w", err)
		}
		if _, err := out.Write(packet); err != nil {
			return frameCount, fmt.Errorf("write frame: %w", err)
		}
		frameCount++
	}

	return frameCount, nil
}

func ParseTime(s string) int {
	if s == "" {
		return 0
	}
	sec, err := strconv.Atoi(s)
	if err == nil {
		return sec
	}
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2:
		m, _ := strconv.Atoi(parts[0])
		s, _ := strconv.Atoi(parts[1])
		return m*60 + s
	case 3:
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		s, _ := strconv.Atoi(parts[2])
		return h*3600 + m*60 + s
	default:
		return 0
	}
}
