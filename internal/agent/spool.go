package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

// SpoolDir holds results that could not be delivered to the server.
const SpoolDir = SupportDir + "/spool"

type spoolItem struct {
	Kind      string                      `json:"kind"` // task | policy
	ID        string                      `json:"id"`
	Task      *model.AgentTaskResult      `json:"task,omitempty"`
	Policy    *model.AgentPolicyLogSubmit `json:"policy,omitempty"`
	CreatedAt time.Time                   `json:"created_at"`
}

func spoolWrite(item spoolItem) {
	os.MkdirAll(SpoolDir, 0o700)
	b, err := json.Marshal(item)
	if err != nil {
		return
	}
	name := filepath.Join(SpoolDir, time.Now().UTC().Format("20060102T150405.000000000")+"-"+item.Kind+".json")
	os.WriteFile(name, b, 0o600)
}

// Spooler retries undelivered results.
type Spooler struct {
	client *Client
	log    *slog.Logger
}

// Flush attempts delivery of every spooled item, oldest first. Items older than 7 days are dropped.
func (s *Spooler) Flush(ctx context.Context) {
	entries, err := os.ReadDir(SpoolDir)
	if err != nil || len(entries) == 0 {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		path := filepath.Join(SpoolDir, n)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var item spoolItem
		if json.Unmarshal(b, &item) != nil || time.Since(item.CreatedAt) > 7*24*time.Hour {
			os.Remove(path)
			continue
		}
		var derr error
		switch item.Kind {
		case "task":
			if item.Task != nil {
				derr = s.client.SubmitTaskResult(ctx, item.ID, *item.Task)
			}
		case "policy":
			if item.Policy != nil {
				derr = s.client.SubmitPolicyLog(ctx, item.ID, *item.Policy)
			}
		}
		if derr != nil {
			var ae *apiError
			if asAPIError(derr, &ae) && ae.Status == 404 {
				os.Remove(path) // server no longer knows this item
				continue
			}
			s.log.Debug("spool delivery failed; will retry", "item", n, "err", derr)
			return // server unreachable; keep order, try later
		}
		s.log.Info("delivered spooled result", "item", n)
		os.Remove(path)
	}
}
