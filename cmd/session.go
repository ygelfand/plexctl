package cmd

import (
	"context"
	"fmt"

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

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active playback sessions",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		res, err := client.SDK.Status.ListSessions(ctx)
		if err != nil {
			return err
		}

		if res.Object == nil || res.Object.MediaContainer == nil || len(res.Object.MediaContainer.Metadata) == 0 {
			fmt.Println("No active sessions.")
			return nil
		}

		var sessions []presenters.SessionMetadata
		for _, m := range res.Object.MediaContainer.Metadata {
			user := "Unknown"
			if m.User != nil {
				user = ui.PtrToString(m.User.Title)
			}
			player := "Unknown"
			state := "Unknown"
			if m.Player != nil {
				player = ui.PtrToString(m.Player.Title)
				state = ui.PtrToString(m.Player.State)
			}
			id := ""
			if m.Session != nil {
				id = ui.PtrToString(m.Session.ID)
			}

			sessions = append(sessions, presenters.SessionMetadata{
				ID:     id,
				User:   user,
				Player: player,
				Title:  m.Title,
				State:  state,
			})
		}

		return commands.Print(&presenters.SessionsPresenter{
			Sessions: sessions,
			RawData:  res.Object.MediaContainer.Metadata,
		}, opts)
	}),
}

var sessionShowCmd = &cobra.Command{
	Use:   "show [session_id]",
	Short: "Show detailed information for a playback session",
	Args:  cobra.MaximumNArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		res, err := client.SDK.Status.ListSessions(ctx)
		if err != nil {
			return err
		}

		if res.Object == nil || res.Object.MediaContainer == nil || len(res.Object.MediaContainer.Metadata) == 0 {
			fmt.Println("No active sessions.")
			return nil
		}

		var sessionID string
		if len(args) > 0 {
			sessionID = args[0]
		} else {
			var options []struct{ Title, Desc, Value string }
			for _, m := range res.Object.MediaContainer.Metadata {
				user := "Unknown"
				if m.User != nil {
					user = ui.PtrToString(m.User.Title)
				}
				player := "Unknown"
				state := "Unknown"
				if m.Player != nil {
					player = ui.PtrToString(m.Player.Title)
					state = ui.PtrToString(m.Player.State)
				}
				id := ""
				if m.Session != nil {
					id = ui.PtrToString(m.Session.ID)
				}

				options = append(options, struct{ Title, Desc, Value string }{
					Title: m.Title,
					Desc:  fmt.Sprintf("User: %s | Player: %s (%s)", user, player, state),
					Value: id,
				})
			}

			sessionID, err = ui.SelectOption("Select a session to show", options)
			if err != nil {
				return err
			}
		}

		for _, m := range res.Object.MediaContainer.Metadata {

			id := ""

			if m.Session != nil {
				id = ui.PtrToString(m.Session.ID)
			}

			if id == sessionID {

				user := "Unknown"

				if m.User != nil {
					user = ui.PtrToString(m.User.Title)
				}

				player := "Unknown"

				state := "Unknown"

				if m.Player != nil {

					player = ui.PtrToString(m.Player.Title)

					state = ui.PtrToString(m.Player.State)

				}

				ui.RenderSummary(fmt.Sprintf("Session %s", sessionID), []struct{ Label, Value string }{

					{"Title", m.Title},

					{"User", user},

					{"Player", player},

					{"State", state},
				})

				return nil

			}

		}

		return fmt.Errorf("session %s not found", sessionID)
	}),
}

var sessionStopCmd = &cobra.Command{
	Use:   "stop [session_id]",
	Short: "Terminate an active playback session",
	Args:  cobra.MaximumNArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		res, err := client.SDK.Status.ListSessions(ctx)
		if err != nil {
			return err
		}

		var sessionID string
		if len(args) > 0 {
			sessionID = args[0]
		} else {
			if res.Object == nil || res.Object.MediaContainer == nil || len(res.Object.MediaContainer.Metadata) == 0 {
				fmt.Println("No active sessions to stop.")
				return nil
			}

			var options []struct{ Title, Desc, Value string }
			for _, m := range res.Object.MediaContainer.Metadata {
				user := "Unknown"
				if m.User != nil {
					user = ui.PtrToString(m.User.Title)
				}
				player := "Unknown"
				state := "Unknown"
				if m.Player != nil {
					player = ui.PtrToString(m.Player.Title)
					state = ui.PtrToString(m.Player.State)
				}
				id := ""
				if m.Session != nil {
					id = ui.PtrToString(m.Session.ID)
				}

				options = append(options, struct{ Title, Desc, Value string }{
					Title: m.Title,
					Desc:  fmt.Sprintf("User: %s | Player: %s (%s)", user, player, state),
					Value: id,
				})
			}

			sessionID, err = ui.SelectOption("Select a session to terminate", options)
			if err != nil {
				return err
			}
		}

		reason := "Terminated via plexctl"
		tRes, err := client.SDK.Status.TerminateSession(ctx, operations.TerminateSessionRequest{
			SessionID: sessionID,
			Reason:    &reason,
		})
		if err != nil {
			return err
		}

		if tRes.StatusCode != 200 {
			return fmt.Errorf("failed to terminate session: %d", tRes.StatusCode)
		}

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
