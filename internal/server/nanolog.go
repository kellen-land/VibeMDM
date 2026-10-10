package server

import (
	"fmt"
	"log/slog"

	nanolog "github.com/micromdm/nanolib/log"
)

// slogAdapter adapts slog to NanoMDM's logger interface.
type slogAdapter struct{ l *slog.Logger }

func newNanoLogger(l *slog.Logger) nanolog.Logger { return &slogAdapter{l: l} }

func split(args []interface{}) (string, []any) {
	msg := "nanomdm"
	attrs := make([]any, 0, len(args))
	for i := 0; i+1 < len(args); i += 2 {
		k := fmt.Sprint(args[i])
		if k == "msg" {
			msg = fmt.Sprint(args[i+1])
			continue
		}
		attrs = append(attrs, slog.Any(k, args[i+1]))
	}
	if len(args)%2 == 1 {
		attrs = append(attrs, slog.Any("extra", args[len(args)-1]))
	}
	return msg, attrs
}

func (a *slogAdapter) Info(args ...interface{}) {
	msg, attrs := split(args)
	a.l.Info(msg, attrs...)
}

func (a *slogAdapter) Debug(args ...interface{}) {
	msg, attrs := split(args)
	a.l.Debug(msg, attrs...)
}

func (a *slogAdapter) With(args ...interface{}) nanolog.Logger {
	_, attrs := split(args)
	return &slogAdapter{l: a.l.With(attrs...)}
}
