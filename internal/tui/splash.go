package tui

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image"
	_ "image/png"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/blacktop/go-termimg"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"golang.org/x/term"
)

//go:embed splash.png
var splashImgData []byte

// ShowStandaloneSplash renders the splash screen and performs background initialization
func ShowStandaloneSplash(minDuration time.Duration, resultChan chan<- plex.LoaderResult) {
	slog.Debug("Splash: Initializing")
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		slog.Error("Splash: Failed to get terminal size", "error", err)
		return
	}

	// 1. Prepare Image
	slog.Debug("Splash: Decoding image")
	imgObj, _, err := image.Decode(bytes.NewReader(splashImgData))
	if err != nil {
		slog.Error("Splash: Failed to decode image", "error", err)
		return
	}
	bounds := imgObj.Bounds()
	imgAR := float64(bounds.Dx()) / float64(bounds.Dy())

	img, err := termimg.From(bytes.NewReader(splashImgData))
	if err != nil {
		slog.Error("Splash: Failed to create termimg", "error", err)
		return
	}

	targetHeight := int(float64(height) * 0.5)
	if targetHeight < 5 {
		targetHeight = 5
	}
	targetWidth := int(float64(targetHeight) * imgAR * 2.0)

	if targetWidth > int(float64(width)*0.8) {
		targetWidth = int(float64(width) * 0.8)
		targetHeight = int(float64(targetWidth) / (imgAR * 2.0))
	}

	slog.Debug("Splash: Rendering image", "width", targetWidth, "height", targetHeight)
	rendered, err := img.
		Width(targetWidth).
		Height(targetHeight).
		Scale(termimg.ScaleFit).
		Render()

	if err != nil {
		slog.Error("Splash: Failed to render image", "error", err)
		return
	}

	x := (width - targetWidth) / 2
	y := (height - targetHeight) / 2

	// 2. Start Worker
	slog.Debug("Splash: Starting data loader")
	updates := make(chan interface{}, 10)
	go plex.LoadData(context.Background(), updates)

	// 3. Render Loop
	fmt.Print("\033[H\033[2J") // Initial Clear
	fmt.Printf("\033[%d;%dH%s", y, x, rendered)

	startTime := time.Now()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	currentStatus := "Initializing..."
	var finalResult plex.LoaderResult
	isDone := false

	slog.Debug("Splash: Starting event loop")
	for {
		select {
		case msg := <-updates:
			switch v := msg.(type) {
			case plex.ProgressUpdate:
				slog.Log(context.Background(), config.LevelTrace, "Splash: Progress update", "msg", v.Message)
				currentStatus = v.Message
			case plex.LoaderResult:
				slog.Debug("Splash: Loader finished")
				finalResult = v
				isDone = true
			case error:
				slog.Error("Splash: Loader fatal error", "error", v)
				fmt.Printf("\nError: %v\n", v)
				os.Exit(1)
			}
		case <-ticker.C:
			// Stylized status update
			fmt.Printf("\033[%d;1H\033[2K%s", height-2, centerText(currentStatus, width))

			// Check if we can exit
			if isDone && time.Since(startTime) >= minDuration {
				slog.Debug("Splash: Exit conditions met")
				fmt.Print("\033[H\033[2J") // Clear for TUI
				resultChan <- finalResult
				return
			}
		}
	}
}

func centerText(text string, width int) string {
	padding := (width - len(text)) / 2
	if padding < 0 {
		padding = 0
	}
	return strings.Repeat(" ", padding) + text
}
