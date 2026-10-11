package store

import (
	"context"
	"strings"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

const mobileProfileCols = `id, name, description, category, identifier, uuid, payload, payloads, scope, created_at, updated_at`

func scanMobileProfile(row interface{ Scan(...any) error }) (*model.MobileConfigProfile, error) {
	var p model.MobileConfigProfile
	var payloads db.JSON[[]model.MobilePayloadValues]
	var scope db.JSON[model.MobileScope]
	if err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Category, &p.Identifier, &p.UUID, &p.Payload, &payloads, &scope, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	p.Payloads = payloads.V
	p.Scope = scope.V
	p.Scope.Normalize()
	return &p, nil
}

// payloadsArg stores editor payloads, or NULL for an uploaded profile.
func payloadsArg(p []model.MobilePayloadValues) any {
	if p == nil {
		return nil
	}
	return jsonArg(p)
}

// ListMobileProfiles returns mobile profiles without their XML.
func (s *Store) ListMobileProfiles(ctx context.Context) ([]model.MobileConfigProfile, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+mobileProfileCols+` FROM mobile_config_profiles ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.MobileConfigProfile{}
	for rows.Next() {
		p, err := scanMobileProfile(rows)
		if err != nil {
			return nil, err
		}
		p.Payload = ""
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetMobileProfile returns a mobile profile with its XML.
func (s *Store) GetMobileProfile(ctx context.Context, id string) (*model.MobileConfigProfile, error) {
	return scanMobileProfile(s.DB.QueryRowContext(ctx, `SELECT `+mobileProfileCols+` FROM mobile_config_profiles WHERE id::text = $1`, id))
}

// CreateMobileProfile inserts a mobile profile.
func (s *Store) CreateMobileProfile(ctx context.Context, p model.MobileConfigProfile) (*model.MobileConfigProfile, error) {
	p.Scope.Normalize()
	return scanMobileProfile(s.DB.QueryRowContext(ctx, `INSERT INTO mobile_config_profiles (name, description, category, identifier, uuid, payload, payloads, scope)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb) RETURNING `+mobileProfileCols,
		p.Name, p.Description, p.Category, p.Identifier, p.UUID, p.Payload, payloadsArg(p.Payloads), jsonArg(p.Scope)))
}

// UpdateMobileProfile updates a mobile profile.
func (s *Store) UpdateMobileProfile(ctx context.Context, p model.MobileConfigProfile) (*model.MobileConfigProfile, error) {
	p.Scope.Normalize()
	return scanMobileProfile(s.DB.QueryRowContext(ctx, `UPDATE mobile_config_profiles SET name=$2, description=$3, category=$4, identifier=$5, uuid=$6,
		payload=$7, payloads=$8::jsonb, scope=$9::jsonb, updated_at=now() WHERE id::text=$1 RETURNING `+mobileProfileCols,
		p.ID, p.Name, p.Description, p.Category, p.Identifier, p.UUID, p.Payload, payloadsArg(p.Payloads), jsonArg(p.Scope)))
}

// DeleteMobileProfile removes a mobile profile (and its deployment rows).
func (s *Store) DeleteMobileProfile(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM mobile_config_profiles WHERE id::text = $1`, id)
	return err
}

// UpsertMobileDeployment records a mobile profile's state on a device.
func (s *Store) UpsertMobileDeployment(ctx context.Context, profileID, deviceID, status, commandUUID, errMsg string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO mobile_profile_deployments (profile_id, mobile_device_id, status, command_uuid, error) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (profile_id, mobile_device_id) DO UPDATE SET status = EXCLUDED.status, command_uuid = EXCLUDED.command_uuid, error = EXCLUDED.error, updated_at = now()`,
		profileID, deviceID, status, commandUUID, errMsg)
	return err
}

// SetMobileDeploymentStatusByCommand updates a mobile deployment from an InstallProfile/RemoveProfile result.
func (s *Store) SetMobileDeploymentStatusByCommand(ctx context.Context, commandUUID, status, errMsg string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE mobile_profile_deployments SET status=$2, error=$3, updated_at=now() WHERE command_uuid=$1`, commandUUID, status, errMsg)
	return err
}

// DeleteMobileDeployment removes a deployment row.
func (s *Store) DeleteMobileDeployment(ctx context.Context, profileID, deviceID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM mobile_profile_deployments WHERE profile_id::text=$1 AND mobile_device_id::text=$2`, profileID, deviceID)
	return err
}

// ListMobileDeployments lists deployments for a profile and/or a device (either may be empty).
func (s *Store) ListMobileDeployments(ctx context.Context, profileID, deviceID string) ([]model.MobileProfileDeployment, error) {
	var where []string
	var args []any
	if profileID != "" {
		args = append(args, profileID)
		where = append(where, "d.profile_id::text = $"+itoa(len(args)))
	}
	if deviceID != "" {
		args = append(args, deviceID)
		where = append(where, "d.mobile_device_id::text = $"+itoa(len(args)))
	}
	w := ""
	if len(where) > 0 {
		w = " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT d.profile_id, p.name, d.mobile_device_id, m.name, d.status, d.command_uuid, d.error, d.updated_at
		FROM mobile_profile_deployments d JOIN mobile_config_profiles p ON p.id = d.profile_id JOIN mobile_devices m ON m.id = d.mobile_device_id`+w+
		` ORDER BY d.updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.MobileProfileDeployment{}
	for rows.Next() {
		var d model.MobileProfileDeployment
		if err := rows.Scan(&d.ProfileID, &d.ProfileName, &d.MobileDeviceID, &d.MobileDeviceName, &d.Status, &d.CommandUUID, &d.Error, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MobileDeploymentStatusCounts summarizes deployment statuses for every mobile profile.
func (s *Store) MobileDeploymentStatusCounts(ctx context.Context) (map[string]map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT profile_id, status, count(*) FROM mobile_profile_deployments GROUP BY 1, 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]int{}
	for rows.Next() {
		var id, st string
		var n int
		if err := rows.Scan(&id, &st, &n); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[string]int{}
		}
		out[id][st] = n
	}
	return out, rows.Err()
}

// mobileSelectionIDs resolves a mobile scope selection to device IDs.
func (s *Store) mobileSelectionIDs(ctx context.Context, sel model.MobileScopeSelection) (map[string]bool, error) {
	set := map[string]bool{}
	for _, id := range sel.MobileDeviceIDs {
		set[id] = true
	}
	for _, g := range sel.GroupIDs {
		ids, err := s.MobileGroupMemberIDs(ctx, g)
		if err != nil {
			if err == ErrNotFound {
				continue // a deleted group matches nothing
			}
			return nil, err
		}
		for _, id := range ids {
			set[id] = true
		}
	}
	if len(sel.Buildings)+len(sel.Departments) > 0 {
		rows, err := s.DB.QueryContext(ctx, `SELECT id FROM mobile_devices WHERE lower(building) = ANY($1) OR lower(department) = ANY($2)`,
			lowered(sel.Buildings), lowered(sel.Departments))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			set[id] = true
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return set, nil
}

// ResolveMobileScope returns the mobile device IDs a mobile object applies to.
func (s *Store) ResolveMobileScope(ctx context.Context, sc model.MobileScope) ([]string, error) {
	sc.Normalize()
	base := map[string]bool{}
	if sc.AllMobileDevices {
		rows, err := s.DB.QueryContext(ctx, `SELECT id FROM mobile_devices`)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			base[id] = true
		}
		rows.Close()
	} else {
		var err error
		if base, err = s.mobileSelectionIDs(ctx, sc.Targets); err != nil {
			return nil, err
		}
	}
	if !sc.Exclusions.IsEmpty() {
		excl, err := s.mobileSelectionIDs(ctx, sc.Exclusions)
		if err != nil {
			return nil, err
		}
		for id := range excl {
			delete(base, id)
		}
	}
	out := make([]string, 0, len(base))
	for id := range base {
		out = append(out, id)
	}
	return out, nil
}
