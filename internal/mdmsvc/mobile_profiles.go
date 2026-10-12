package mdmsvc

import (
	"context"

	"github.com/vibemdm/vibemdm/internal/commands"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/profilevars"
)

// InstallMobileProfile queues an InstallProfile for an iPhone or iPad and records the deployment.
func (c *Commander) InstallMobileProfile(ctx context.Context, d *model.MobileDevice, prof *model.MobileConfigProfile, createdBy string) error {
	cmd, err := commands.InstallProfile([]byte(profilevars.Expand(prof.Payload, profilevars.ForMobileDevice(d))))
	if err != nil {
		return err
	}
	if err := c.st.UpsertMobileDeployment(ctx, prof.ID, d.ID, "pending", cmd.CommandUUID, ""); err != nil {
		return err
	}
	if _, err := c.EnqueueMobile(ctx, d, cmd, "profile_install", prof.ID, createdBy); err != nil {
		c.st.UpsertMobileDeployment(ctx, prof.ID, d.ID, "failed", cmd.CommandUUID, err.Error())
		return err
	}
	return nil
}

// RemoveMobileProfile queues a RemoveProfile for an iPhone or iPad and records the deployment.
func (c *Commander) RemoveMobileProfile(ctx context.Context, d *model.MobileDevice, prof *model.MobileConfigProfile, createdBy string) error {
	cmd, err := commands.RemoveProfile(prof.Identifier)
	if err != nil {
		return err
	}
	if err := c.st.UpsertMobileDeployment(ctx, prof.ID, d.ID, "removing", cmd.CommandUUID, ""); err != nil {
		return err
	}
	if _, err := c.EnqueueMobile(ctx, d, cmd, "profile_remove", prof.ID, createdBy); err != nil {
		c.st.UpsertMobileDeployment(ctx, prof.ID, d.ID, "failed", cmd.CommandUUID, err.Error())
		return err
	}
	return nil
}

// SyncMobileProfile reconciles a mobile profile's scope: installs where missing
// (or everywhere when reinstall is set), removes where out of scope.
func (c *Commander) SyncMobileProfile(ctx context.Context, prof *model.MobileConfigProfile, createdBy string, reinstall bool) (installed, removed int, err error) {
	targets, err := c.st.ResolveMobileScope(ctx, prof.Scope)
	if err != nil {
		return 0, 0, err
	}
	inScope := map[string]bool{}
	for _, id := range targets {
		inScope[id] = true
	}
	deps, err := c.st.ListMobileDeployments(ctx, prof.ID, "")
	if err != nil {
		return 0, 0, err
	}
	deployed := map[string]model.MobileProfileDeployment{}
	for _, d := range deps {
		deployed[d.MobileDeviceID] = d
	}
	for _, id := range targets {
		d, err := c.st.GetMobileDevice(ctx, id)
		if err != nil || !d.MDMEnrolled {
			continue
		}
		dep, has := deployed[id]
		if has && !reinstall && (dep.Status == "installed" || dep.Status == "pending") {
			continue
		}
		if err := c.InstallMobileProfile(ctx, d, prof, createdBy); err == nil {
			installed++
		}
	}
	for _, dep := range deps {
		if inScope[dep.MobileDeviceID] || dep.Status == "removed" || dep.Status == "removing" {
			continue
		}
		d, err := c.st.GetMobileDevice(ctx, dep.MobileDeviceID)
		if err != nil || !d.MDMEnrolled {
			c.st.DeleteMobileDeployment(ctx, prof.ID, dep.MobileDeviceID)
			continue
		}
		if err := c.RemoveMobileProfile(ctx, d, prof, createdBy); err == nil {
			removed++
		}
	}
	return installed, removed, nil
}

// SyncMobileProfilesForDevice installs every mobile profile scoped to a newly enrolled device.
func (c *Commander) SyncMobileProfilesForDevice(ctx context.Context, d *model.MobileDevice, createdBy string) error {
	profiles, err := c.st.ListMobileProfiles(ctx)
	if err != nil {
		return err
	}
	for _, p := range profiles {
		ids, err := c.st.ResolveMobileScope(ctx, p.Scope)
		if err != nil {
			return err
		}
		in := false
		for _, id := range ids {
			if id == d.ID {
				in = true
				break
			}
		}
		if !in {
			continue
		}
		full, err := c.st.GetMobileProfile(ctx, p.ID)
		if err != nil {
			continue
		}
		if err := c.InstallMobileProfile(ctx, d, full, createdBy); err != nil {
			c.log.Warn("install mobile profile on enrollment", "profile", p.Name, "err", err)
		}
	}
	return nil
}
