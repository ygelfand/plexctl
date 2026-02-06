package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
	"github.com/ygelfand/plexctl/internal/ui"
)

var tasksCmd = &cobra.Command{
	Use:     "tasks",
	Short:   "Manage background butler tasks",
	GroupID: "media",
}

var tasksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all butler tasks",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		slog.Debug("SDK: Fetching butler tasks")
		res, err := client.SDK.Butler.GetTasks(ctx)
		if err != nil {
			slog.Error("SDK: Failed to get butler tasks", "error", err)
			return err
		}

		if res.Object == nil || res.Object.ButlerTasks == nil {
			slog.Debug("SDK: No butler tasks found")
			fmt.Println("No butler tasks found.")
			return nil
		}

		slog.Debug("SDK: Found butler tasks", "count", len(res.Object.ButlerTasks.ButlerTask))

		headers := []string{"NAME", "TITLE", "INTERVAL", "ENABLED"}
		var rows [][]string
		for _, task := range res.Object.ButlerTasks.ButlerTask {
			name := ""
			if task.Name != nil {
				name = *task.Name
			}
			title := ""
			if task.Title != nil {
				title = *task.Title
			}
			interval := ""
			if task.Interval != nil {
				interval = fmt.Sprintf("%d days", *task.Interval)
			}
			enabled := "false"
			if task.Enabled != nil && *task.Enabled {
				enabled = "true"
			}
			rows = append(rows, []string{name, title, interval, enabled})
		}

		return commands.Print(presenters.SimplePresenter{
			T:       "Butler Tasks",
			H:       headers,
			R:       rows,
			RawData: res.Object.ButlerTasks.ButlerTask,
		}, opts)
	}),
}

var tasksStartCmd = &cobra.Command{
	Use:   "start [task_name]",
	Short: "Start a butler task",
	Args:  cobra.MaximumNArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		var taskName string
		if len(args) > 0 {
			taskName = args[0]
		} else {
			slog.Debug("SDK: Fetching tasks for interactive selection")
			res, err := client.SDK.Butler.GetTasks(ctx)
			if err != nil {
				slog.Error("SDK: Failed to get tasks for selection", "error", err)
				return err
			}
			var options []struct{ Title, Desc, Value string }
			for _, t := range res.Object.ButlerTasks.ButlerTask {
				options = append(options, struct{ Title, Desc, Value string }{
					Title: *t.Title,
					Desc:  *t.Description,
					Value: *t.Name,
				})
			}
			taskName, err = ui.SelectOption("Select a task to start", options)
			if err != nil {
				return err
			}
		}

		slog.Debug("SDK: Starting task", "task_name", taskName)
		_, err := client.SDK.Butler.StartTask(ctx, operations.StartTaskRequest{
			ButlerTask: operations.PathParamButlerTask(taskName),
		})
		if err != nil {
			slog.Error("SDK: Start task failed", "task_name", taskName, "error", err)
			return err
		}
		slog.Debug("SDK: Task started successfully", "task_name", taskName)
		fmt.Printf("Task %s started.\n", taskName)
		return nil
	}),
}

var tasksStopCmd = &cobra.Command{
	Use:   "stop [task_name]",
	Short: "Stop a butler task",
	Args:  cobra.MaximumNArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		var taskName string
		if len(args) > 0 {
			taskName = args[0]
		} else {
			slog.Debug("SDK: Fetching tasks for interactive stop selection")
			res, err := client.SDK.Butler.GetTasks(ctx)
			if err != nil {
				slog.Error("SDK: Failed to get tasks for stop selection", "error", err)
				return err
			}
			var options []struct{ Title, Desc, Value string }
			for _, t := range res.Object.ButlerTasks.ButlerTask {
				options = append(options, struct{ Title, Desc, Value string }{
					Title: *t.Title,
					Desc:  *t.Description,
					Value: *t.Name,
				})
			}
			taskName, err = ui.SelectOption("Select a task to stop", options)
			if err != nil {
				return err
			}
		}

		slog.Debug("SDK: Stopping task", "task_name", taskName)
		_, err := client.SDK.Butler.StopTask(ctx, operations.StopTaskRequest{
			ButlerTask: operations.ButlerTask(taskName),
		})
		if err != nil {
			slog.Error("SDK: Stop task failed", "task_name", taskName, "error", err)
			return err
		}
		slog.Debug("SDK: Task stopped successfully", "task_name", taskName)
		fmt.Printf("Task %s stopped.\n", taskName)
		return nil
	}),
}

func init() {
	rootCmd.AddCommand(tasksCmd)
	tasksCmd.AddCommand(tasksListCmd)
	tasksCmd.AddCommand(tasksStartCmd)
	tasksCmd.AddCommand(tasksStopCmd)
}
