package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
	"github.com/ygelfand/plexctl/internal/ui"
)

var sessionCmd = &cobra.Command{
	Use:     "session",
	Short:   "Manage active playback sessions",
	GroupID: "media",
}

type sessionInfo struct {
	ID     string `json:"id"`
	User   string `json:"user"`
	Player string `json:"player"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

type sessionResponse struct {
	MediaContainer struct {
		Metadata []struct {
			Title string `json:"title"`
			User  struct {
				Title string `json:"title"`
			} `json:"User"`
			Player struct {
				Title string `json:"title"`
				State string `json:"state"`
			} `json:"Player"`
			Session struct {
				ID string `json:"id"`
			} `json:"Session"`
		} `json:"Metadata"`
	} `json:"MediaContainer"`
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active playback sessions",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		slog.Debug("SDK: Fetching sessions")
		res, err := client.SDK.Status.ListSessions(ctx)
		if err != nil {
			slog.Error("SDK: Failed to list sessions", "error", err)
			return err
		}

		defer res.RawResponse.Body.Close()
		body, err := io.ReadAll(res.RawResponse.Body)
		if err != nil {
			slog.Error("SDK: Failed to read sessions response", "error", err)
			return err
		}

		var sRes sessionResponse
		if err := json.Unmarshal(body, &sRes); err != nil {
			slog.Error("SDK: Failed to parse sessions JSON", "error", err)
			return err
		}

		var sessions []sessionInfo
		for _, m := range sRes.MediaContainer.Metadata {
			sessions = append(sessions, sessionInfo{
				ID:     m.Session.ID,
				User:   m.User.Title,
				Player: m.Player.Title,
				Title:  m.Title,
				State:  m.Player.State,
			})
		}

		if len(sessions) == 0 {
			slog.Debug("SDK: No active sessions found")
			fmt.Println("No active sessions.")
			return nil
		}

		slog.Debug("SDK: Found sessions", "count", len(sessions))

		// Use OutputData for consistent formatting
		headers := []string{"ID", "USER", "PLAYER", "TITLE", "STATE"}
		var rows [][]string
		for _, s := range sessions {
			rows = append(rows, []string{s.ID, s.User, s.Player, s.Title, s.State})
		}

		return commands.Print(presenters.SimplePresenter{
			T: "Active Sessions",

			H: headers,

			R: rows,

			RawData: sessions,
		}, opts)
	}),
}

var sessionShowCmd = &cobra.Command{
	Use: "show [session_id]",

	Short: "Show detailed information for a playback session",

	Args: cobra.MaximumNArgs(1),

	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		slog.Debug("SDK: Fetching sessions for show")
		res, err := client.SDK.Status.ListSessions(ctx)
		if err != nil {
			slog.Error("SDK: Failed to list sessions for show", "error", err)
			return err
		}

		defer res.RawResponse.Body.Close()

		body, err := io.ReadAll(res.RawResponse.Body)
		if err != nil {
			slog.Error("SDK: Failed to read sessions response for show", "error", err)
			return err
		}

		var sRes sessionResponse

		if err := json.Unmarshal(body, &sRes); err != nil {
			slog.Error("SDK: Failed to parse sessions JSON for show", "error", err)
			return err
		}

		var sessionID string

		if len(args) > 0 {
			sessionID = args[0]
		} else {

			// Interactive selection

			var options []struct{ Title, Desc, Value string }

			for _, m := range sRes.MediaContainer.Metadata {
				options = append(options, struct{ Title, Desc, Value string }{
					Title: m.Title,

					Desc: fmt.Sprintf("User: %s | Player: %s (%s)", m.User.Title, m.Player.Title, m.Player.State),

					Value: m.Session.ID,
				})
			}

			if len(options) == 0 {
				slog.Debug("SDK: No active sessions found for interactive selection")
				fmt.Println("No active sessions.")

				return nil

			}

			sessionID, err = ui.SelectOption("Select a session to show", options)
			if err != nil {
				return err
			}

		}

		// Find the session

		for _, m := range sRes.MediaContainer.Metadata {
			if m.Session.ID == sessionID {
				slog.Debug("SDK: Found session to show", "session_id", sessionID)

				// We can reuse the metadata presenter or simple summary

				ui.RenderSummary(fmt.Sprintf("Session %s", sessionID), []struct{ Label, Value string }{
					{"Title", m.Title},

					{"User", m.User.Title},

					{"Player", m.Player.Title},

					{"State", m.Player.State},
				})

				return nil

			}
		}

		slog.Warn("SDK: Session not found", "session_id", sessionID)
		return fmt.Errorf("session %s not found", sessionID)
	}),
}

var sessionStopCmd = &cobra.Command{
	Use: "stop [session_id]",

	Short: "Terminate an active playback session",

	Args: cobra.MaximumNArgs(1),

	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		slog.Debug("SDK: Fetching sessions for termination check")
		res, err := client.SDK.Status.ListSessions(ctx)
		if err != nil {
			slog.Error("SDK: Failed to list sessions for termination", "error", err)
			return err
		}

		defer res.RawResponse.Body.Close()

		body, err := io.ReadAll(res.RawResponse.Body)
		if err != nil {
			slog.Error("SDK: Failed to read sessions response for termination", "error", err)
			return err
		}

		var sRes sessionResponse

		if err := json.Unmarshal(body, &sRes); err != nil {
			slog.Error("SDK: Failed to parse sessions JSON for termination", "error", err)
			return err
		}

		var sessionID string

		if len(args) > 0 {
			sessionID = args[0]
		} else {

			// Interactive selection

			var options []struct{ Title, Desc, Value string }

			for _, m := range sRes.MediaContainer.Metadata {
				options = append(options, struct{ Title, Desc, Value string }{
					Title: m.Title,

					Desc: fmt.Sprintf("User: %s | Player: %s (%s)", m.User.Title, m.Player.Title, m.Player.State),

					Value: m.Session.ID,
				})
			}

			if len(options) == 0 {
				slog.Debug("SDK: No active sessions found for termination")
				fmt.Println("No active sessions.")

				return nil

			}

			sessionID, err = ui.SelectOption("Select a session to terminate", options)
			if err != nil {
				return err
			}

		}

		reason := "Terminated via plexctl"
		slog.Debug("SDK: Terminating session", "session_id", sessionID)

		tRes, err := client.SDK.Status.TerminateSession(ctx, operations.TerminateSessionRequest{
			SessionID: sessionID,

			Reason: &reason,
		})
		if err != nil {
			slog.Error("SDK: Terminate session failed", "session_id", sessionID, "error", err)
			return err
		}

		if tRes.StatusCode != 200 {
			slog.Error("SDK: Terminate session returned error status", "session_id", sessionID, "status", tRes.StatusCode)
			return fmt.Errorf("failed to terminate session: %d", tRes.StatusCode)
		}

		slog.Debug("SDK: Session terminated successfully", "session_id", sessionID)
		fmt.Printf("Session %s terminated.\n", sessionID)

		return nil
	}),
}

func init() {
	rootCmd.AddCommand(sessionCmd)

	sessionCmd.AddCommand(sessionListCmd)

	sessionCmd.AddCommand(sessionShowCmd)

	sessionCmd.AddCommand(sessionStopCmd)
}
