package cmd

import (
	"testing"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestTaskOptionsHandlesSparseResponses(t *testing.T) {
	for _, tt := range []struct {
		name string
		res  *operations.GetTasksResponse
	}{
		{"missing response", nil},
		{"missing object", &operations.GetTasksResponse{}},
		{"missing task container", &operations.GetTasksResponse{Object: &operations.GetTasksResponseBody{}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if options := taskOptions(tt.res); len(options) != 0 {
				t.Fatalf("unexpected options: %v", options)
			}
		})
	}
	res := &operations.GetTasksResponse{Object: &operations.GetTasksResponseBody{
		ButlerTasks: &operations.ButlerTasks{ButlerTask: []operations.GetTasksButlerTask{
			{},
			{Name: ui.Ptr("")},
			{Name: ui.Ptr("CleanOldBundles")},
			{Name: ui.Ptr("RefreshLibraries"), Title: ui.Ptr("Refresh"), Description: ui.Ptr("Refresh metadata")},
		}},
	}}
	options := taskOptions(res)
	if len(options) != 2 || options[0].Title != "CleanOldBundles" || options[0].Desc != "" || options[1].Title != "Refresh" || options[1].Value != "RefreshLibraries" {
		t.Fatalf("invalid task options: %#v", options)
	}
}
