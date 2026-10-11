//go:build darwin && cgo

package agent

// macOS unified logging through os_log. This file only builds with cgo on a Mac
// (make agent-macos); the cross-compiled agent (CGO_ENABLED=0) uses
// oslog_other.go and logs to the file and the server only.
//
// Messages use the %{public}s format: without it, unified logging stores
// dynamic strings as <private>. Secrets are redacted before they get here.

/*
#include <os/log.h>
#include <stdlib.h>

static os_log_t vibemdm_log_create(const char *subsystem, const char *category) {
	return os_log_create(subsystem, category);
}

static void vibemdm_log(os_log_t log, uint8_t type, const char *msg) {
	os_log_with_type(log, (os_log_type_t)type, "%{public}s", msg);
}
*/
import "C"

import (
	"log/slog"
	"sync"
	"unsafe"
)

const unifiedAvailable = true

var (
	osLogsMu sync.Mutex
	osLogs   = map[string]C.os_log_t{} // one per category; os_log handles are never freed
)

func osLogFor(category string) C.os_log_t {
	osLogsMu.Lock()
	defer osLogsMu.Unlock()
	if l, ok := osLogs[category]; ok {
		return l
	}
	sub := C.CString(LogSubsystem)
	cat := C.CString(category)
	defer C.free(unsafe.Pointer(sub))
	defer C.free(unsafe.Pointer(cat))
	l := C.vibemdm_log_create(sub, cat)
	osLogs[category] = l
	return l
}

// unifiedLog maps slog levels to os_log types. Info is written as "default"
// so it is persisted (os_log "info" records are kept in memory only); warnings
// are default records prefixed with WARN, because os_log has no warning type.
func unifiedLog(category string, l slog.Level, msg string) {
	var typ C.uint8_t
	switch {
	case l < slog.LevelInfo:
		typ = C.OS_LOG_TYPE_DEBUG
	case l < slog.LevelWarn:
		typ = C.OS_LOG_TYPE_DEFAULT
	case l < slog.LevelError:
		typ = C.OS_LOG_TYPE_DEFAULT
		msg = "WARN " + msg
	default:
		typ = C.OS_LOG_TYPE_ERROR
	}
	cmsg := C.CString(msg)
	defer C.free(unsafe.Pointer(cmsg))
	C.vibemdm_log(osLogFor(category), typ, cmsg)
}
