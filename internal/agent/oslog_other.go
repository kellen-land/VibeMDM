//go:build !darwin || !cgo

package agent

import "log/slog"

// unifiedAvailable is false when the agent is built without cgo (the default
// cross-compiled build) or for another OS: records go to the file and the
// server only. Build on a Mac with `make agent-macos` to enable os_log.
const unifiedAvailable = false

func unifiedLog(string, slog.Level, string) {}
