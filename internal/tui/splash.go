package tui

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"golang.org/x/term"
)

//go:embed splash.ans
var splashArt string

const artWidth = 68 // Static width of the art in splash.ans

// ShowStandaloneSplash renders the splash screen and performs background initialization
func ShowStandaloneSplash(minDuration time.Duration, resultChan chan<- plex.LoaderResult) {
	slog.Debug("Splash: Initializing")
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		slog.Error("Splash: Failed to get terminal size", "error", err)
		return
	}

	// 1. Prepare Worker
	slog.Debug("Splash: Starting data loader")
	updates := make(chan interface{}, 10)
	go plex.LoadData(context.Background(), updates)

	// 2. Render Initial Splash
	fmt.Print("\033[H\033[2J") // Clear screen

	lines := strings.Split(splashArt, "\n")
	artHeight := len(lines)
	startY := (height - artHeight) / 2
	if startY < 1 {
		startY = 1
	}

	// Calculate horizontal centering
	startX := (width - artWidth) / 2
	if startX < 1 {
		startX = 1
	}
	padding := strings.Repeat(" ", startX)

	// Print line by line with calculated left padding
	for i, line := range lines {
		if strings.TrimSpace(line) == "" && !strings.Contains(line, "\033") {
			// Handle truly empty lines (spacing)
			fmt.Printf("\033[%d;1H", startY+i)
			continue
		}
		fmt.Printf("\033[%d;1H%s%s", startY+i, padding, line)
	}

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
