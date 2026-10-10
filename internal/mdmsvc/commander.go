package mdmsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	nanoapi "github.com/micromdm/nanomdm/api"
	"github.com/micromdm/nanomdm/mdm"
	"github.com/micromdm/nanomdm/push"
	"github.com/micromdm/nanomdm/storage"

	"github.com/vibemdm/vibemdm/internal/commands"
	"github.com/vibemdm/vibemdm/internal/inspector"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// ErrNotEnrolled is returned when a computer has no MDM enrollment.
var ErrNotEnrolled = errors.New("computer is not enrolled in MDM")

// Commander enqueues MDM commands with VibeMDM metadata and pushes.
type Commander struct {
	st     *store.Store
	pe     *nanoapi.PushEnqueuer
	pusher push.Pusher
	log    *slog.Logger
}

// NewCommander creates a Commander.
func NewCommander(st *store.Store, enqueuer storage.CommandEnqueuer, pusher push.Pusher, logger *slog.Logger) (*Commander, error) {
	pe, err := nanoapi.NewPushEnqueuer(enqueuer, pusher)
	if err != nil {
		return nil, err
	}
	return &Commander{st: st, pe: pe, pusher: pusher, log: logger}, nil
}

// Enqueue queues a command for a computer and sends a push.
func (c *Commander) Enqueue(ctx context.Context, comp *model.Computer, cmd *mdm.Command, kind, refID, createdBy string) (*nanoapi.APIResult, error) {
	if comp == nil || comp.UDID == "" || !comp.MDMEnrolled {
		return nil, ErrNotEnrolled
	}
	if err := c.st.InsertCommandMeta(ctx, cmd.CommandUUID, comp.ID, comp.UDID, cmd.Command.RequestType, kind, refID, createdBy); err != nil {
		return nil, err
	}
	res, _, err := c.pe.EnqueueWithPush(ctx, cmd, []string{comp.UDID}, false)
	if err != nil {
		return res, err
	}
	if res != nil && res.EnqueueError != nil {
		return res, fmt.Errorf("enqueue: %w", res.EnqueueError)
	}
	c.event(ctx, cmd.CommandUUID, comp.UDID, inspector.Queued, map[string]any{"request_type": cmd.Command.RequestType, "kind": kind, "by": createdBy})
	pushErr := ""
	pushID := ""
	if res != nil {
		if res.PushError != nil {
			pushErr = res.PushError.Error()
		}
		if st, ok := res.Status[comp.UDID]; ok {
			pushID = st.PushID
			if st.PushError != nil {
				pushErr = st.PushError.Error()
			}
		}
	}
	if pushErr != "" {
		c.log.Warn("push failed after enqueue", "err", pushErr, "computer", comp.Name)
		c.event(ctx, cmd.CommandUUID, comp.UDID, inspector.PushFailed, map[string]any{"error": pushErr})
	} else {
		c.event(ctx, cmd.CommandUUID, comp.UDID, inspector.PushSent, map[string]any{"apns_id": pushID})
	}
	return res, nil
}

func (c *Commander) event(ctx context.Context, uuid, enrollmentID, event string, details map[string]any) {
	if err := c.st.RecordCommandEvent(ctx, uuid, enrollmentID, event, details); err != nil {
		c.log.Debug("recording command event", "err", err)
	}
}

// Push sends a blank push to a computer.
func (c *Commander) Push(ctx context.Context, comp *model.Computer) (*nanoapi.APIResult, error) {
	if comp == nil || comp.UDID == "" || !comp.MDMEnrolled {
		return nil, ErrNotEnrolled
	}
	res, _, err := c.pe.Push(ctx, []string{comp.UDID})
	return res, err
}

// RequestInventory queues the standard inventory command set.
func (c *Commander) RequestInventory(ctx context.Context, comp *model.Computer, createdBy string) error {
	builders := []func() (*mdm.Command, error){commands.DeviceInformation, commands.SecurityInfo, commands.ProfileList, commands.InstalledApplicationList}
	var firstErr error
	for _, b := range builders {
		cmd, err := b()
		if err != nil {
			return err
		}
		if _, err := c.Enqueue(ctx, comp, cmd, "inventory", "", createdBy); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// InstallProfile queues an InstallProfile and records the deployment.
func (c *Commander) InstallProfile(ctx context.Context, comp *model.Computer, prof *model.ConfigProfile, createdBy string) error {
	cmd, err := commands.InstallProfile([]byte(prof.Payload))
	if err != nil {
		return err
	}
	if err := c.st.UpsertDeployment(ctx, prof.ID, comp.ID, "pending", cmd.CommandUUID, ""); err != nil {
		return err
	}
	if _, err := c.Enqueue(ctx, comp, cmd, "profile_install", prof.ID, createdBy); err != nil {
		c.st.UpsertDeployment(ctx, prof.ID, comp.ID, "failed", cmd.CommandUUID, err.Error())
		return err
	}
	return nil
}

// RemoveProfile queues a RemoveProfile and records the deployment.
func (c *Commander) RemoveProfile(ctx context.Context, comp *model.Computer, prof *model.ConfigProfile, createdBy string) error {
	cmd, err := commands.RemoveProfile(prof.Identifier)
	if err != nil {
		return err
	}
	if err := c.st.UpsertDeployment(ctx, prof.ID, comp.ID, "removing", cmd.CommandUUID, ""); err != nil {
		return err
	}
	if _, err := c.Enqueue(ctx, comp, cmd, "profile_remove", prof.ID, createdBy); err != nil {
		c.st.UpsertDeployment(ctx, prof.ID, comp.ID, "failed", cmd.CommandUUID, err.Error())
		return err
	}
	return nil
}

// SyncProfile reconciles a profile's scope: installs where missing, removes where out of scope.
func (c *Commander) SyncProfile(ctx context.Context, prof *model.ConfigProfile, createdBy string, reinstall bool) (installed, removed int, err error) {
	targets, err := c.st.ResolveScope(ctx, prof.Scope)
	if err != nil {
		return 0, 0, err
	}
	inScope := map[string]bool{}
	for _, id := range targets {
		inScope[id] = true
	}
	deps, err := c.st.ListDeployments(ctx, prof.ID, "")
	if err != nil {
		return 0, 0, err
	}
	deployed := map[string]model.ProfileDeployment{}
	for _, d := range deps {
		deployed[d.ComputerID] = d
	}
	for _, id := range targets {
		comp, err := c.st.GetComputer(ctx, id)
		if err != nil || !comp.MDMEnrolled {
			continue
		}
		d, has := deployed[id]
		if has && !reinstall && (d.Status == "installed" || d.Status == "pending") {
			continue
		}
		if err := c.InstallProfile(ctx, comp, prof, createdBy); err == nil {
			installed++
		}
	}
	for _, d := range deps {
		if inScope[d.ComputerID] || d.Status == "removed" || d.Status == "removing" {
			continue
		}
		comp, err := c.st.GetComputer(ctx, d.ComputerID)
		if err != nil || !comp.MDMEnrolled {
			c.st.DeleteDeployment(ctx, prof.ID, d.ComputerID)
			continue
		}
		if err := c.RemoveProfile(ctx, comp, prof, createdBy); err == nil {
			removed++
		}
	}
	return installed, removed, nil
}

// SyncProfilesForComputer installs all scoped profiles to one computer.
func (c *Commander) SyncProfilesForComputer(ctx context.Context, comp *model.Computer, createdBy string) error {
	profiles, err := c.st.ListProfiles(ctx)
	if err != nil {
		return err
	}
	cache := store.NewMembershipCache()
	for _, p := range profiles {
		in, err := c.st.InScopeComputer(ctx, comp, p.Scope, cache)
		if err != nil || !in {
			continue
		}
		full, err := c.st.GetProfile(ctx, p.ID)
		if err != nil {
			continue
		}
		if err := c.InstallProfile(ctx, comp, full, createdBy); err != nil {
			c.log.Warn("install profile on enrollment", "profile", p.Name, "err", err)
		}
	}
	return nil
}
