package commands

import "context"

func stopCommand() Definition {
	return Definition{
		Name:        "stop",
		Description: "Cancel the currently running task",
		Usage:       "/stop",
		Aliases:     []string{"cancel", "abort"},
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.StopTask == nil {
				return req.Reply(unavailableMsg)
			}
			if rt.StopTask() {
				return req.Reply("Task stopped.")
			}
			return req.Reply("No active task to stop.")
		},
	}
}
