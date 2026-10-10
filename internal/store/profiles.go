package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

const profileCols = `id, name, description, category, identifier, uuid, level, payload, scope, created_at, updated_at, schema_source, schema_values`

func scanProfile(row interface{ Scan(...any) error }) (*model.ConfigProfile, error) {
	var p model.ConfigProfile
	var scope db.JSON[model.Scope]
	var vals db.JSON[map[string]any]
	if err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Category, &p.Identifier, &p.UUID, &p.Level, &p.Payload, &scope, &p.CreatedAt, &p.UpdatedAt,
		&p.SchemaSource, &vals); err != nil {
		return nil, wrapNotFound(err)
	}
	p.Scope = scope.V
	if p.SchemaSource != "" {
		p.SchemaValues = vals.V
	}
	p.Scope.Normalize()
	return &p, nil
}

// ListProfiles returns profiles (without payload).
func (s *Store) ListProfiles(ctx context.Context) ([]model.ConfigProfile, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+profileCols+` FROM config_profiles ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ConfigProfile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		p.Payload = ""
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetProfile returns a profile with payload.
func (s *Store) GetProfile(ctx context.Context, id string) (*model.ConfigProfile, error) {
	return scanProfile(s.DB.QueryRowContext(ctx, `SELECT `+profileCols+` FROM config_profiles WHERE id = $1`, id))
}

// CreateProfile inserts a profile.
func (s *Store) CreateProfile(ctx context.Context, p model.ConfigProfile) (*model.ConfigProfile, error) {
	p.Scope.Normalize()
	if p.Level != "user" {
		p.Level = "device"
	}
	return scanProfile(s.DB.QueryRowContext(ctx, `INSERT INTO config_profiles (name, description, category, identifier, uuid, level, payload, scope)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb) RETURNING `+profileCols,
		p.Name, p.Description, p.Category, p.Identifier, p.UUID, p.Level, p.Payload, jsonArg(p.Scope)))
}

// UpdateProfile updates a profile.
func (s *Store) UpdateProfile(ctx context.Context, p model.ConfigProfile) (*model.ConfigProfile, error) {
	p.Scope.Normalize()
	if p.Level != "user" {
		p.Level = "device"
	}
	return scanProfile(s.DB.QueryRowContext(ctx, `UPDATE config_profiles SET name=$2, description=$3, category=$4, identifier=$5, uuid=$6, level=$7, payload=$8, scope=$9::jsonb, updated_at=now()
		WHERE id=$1 RETURNING `+profileCols,
		p.ID, p.Name, p.Description, p.Category, p.Identifier, p.UUID, p.Level, p.Payload, jsonArg(p.Scope)))
}

// DeleteProfile removes a profile.
func (s *Store) DeleteProfile(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM config_profiles WHERE id=$1`, id)
	return err
}

// UpsertDeployment records a deployment status.
func (s *Store) UpsertDeployment(ctx context.Context, profileID, computerID, status, commandUUID, errMsg string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO profile_deployments (profile_id, computer_id, status, command_uuid, error) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (profile_id, computer_id) DO UPDATE SET status = EXCLUDED.status, command_uuid = EXCLUDED.command_uuid, error = EXCLUDED.error, updated_at = now()`,
		profileID, computerID, status, commandUUID, errMsg)
	return err
}

// SetDeploymentStatusByCommand updates a deployment by command UUID.
func (s *Store) SetDeploymentStatusByCommand(ctx context.Context, commandUUID, status, errMsg string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE profile_deployments SET status=$2, error=$3, updated_at=now() WHERE command_uuid=$1`, commandUUID, status, errMsg)
	return err
}

// DeleteDeployment removes a deployment row.
func (s *Store) DeleteDeployment(ctx context.Context, profileID, computerID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM profile_deployments WHERE profile_id=$1 AND computer_id=$2`, profileID, computerID)
	return err
}

// ListDeployments lists deployments for a profile or computer (either may be empty).
func (s *Store) ListDeployments(ctx context.Context, profileID, computerID string) ([]model.ProfileDeployment, error) {
	var where []string
	var args []any
	if profileID != "" {
		args = append(args, profileID)
		where = append(where, "d.profile_id = $1")
	}
	if computerID != "" {
		args = append(args, computerID)
		where = append(where, "d.computer_id = $"+itoa(len(args)))
	}
	w := ""
	if len(where) > 0 {
		w = " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT d.profile_id, p.name, d.computer_id, c.name, d.status, d.command_uuid, d.error, d.updated_at
		FROM profile_deployments d JOIN config_profiles p ON p.id = d.profile_id JOIN computers c ON c.id = d.computer_id`+w+` ORDER BY d.updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ProfileDeployment{}
	for rows.Next() {
		var d model.ProfileDeployment
		if err := rows.Scan(&d.ProfileID, &d.ProfileName, &d.ComputerID, &d.ComputerName, &d.Status, &d.CommandUUID, &d.Error, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeploymentStatusCounts summarizes deployment statuses per profile.
func (s *Store) DeploymentStatusCounts(ctx context.Context, profileID string) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT status, count(*) FROM profile_deployments WHERE profile_id = $1 GROUP BY status`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

func itoa(n int) string { return strconv.Itoa(n) }

// GetProfileByIdentifier returns a profile by its PayloadIdentifier.
func (s *Store) GetProfileByIdentifier(ctx context.Context, identifier string) (*model.ConfigProfile, error) {
	return scanProfile(s.DB.QueryRowContext(ctx, `SELECT `+profileCols+` FROM config_profiles WHERE identifier = $1`, identifier))
}

// SetProfileSchema records (or clears, with source "") the schema a profile was built from.
func (s *Store) SetProfileSchema(ctx context.Context, profileID, source string, values map[string]any) error {
	if values == nil {
		values = map[string]any{}
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE config_profiles SET schema_source = $2, schema_values = $3::jsonb WHERE id = $1`, profileID, source, jsonArg(values))
	return err
}

// ---- custom schemas ----

// ListCustomSchemas returns uploaded schemas (without the schema body unless full).
func (s *Store) ListCustomSchemas(ctx context.Context) ([]model.CustomSchema, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, preference_domain, schema, created_by, created_at, updated_at FROM custom_schemas ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CustomSchema{}
	for rows.Next() {
		var c model.CustomSchema
		var raw []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.PreferenceDomain, &raw, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Schema = raw
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCustomSchema loads one schema.
func (s *Store) GetCustomSchema(ctx context.Context, id string) (*model.CustomSchema, error) {
	var c model.CustomSchema
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT id, name, preference_domain, schema, created_by, created_at, updated_at FROM custom_schemas WHERE id = $1`, id).
		Scan(&c.ID, &c.Name, &c.PreferenceDomain, &raw, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, wrapNotFound(err)
	}
	c.Schema = raw
	return &c, nil
}

// SaveCustomSchema creates (empty ID) or updates a schema.
func (s *Store) SaveCustomSchema(ctx context.Context, c model.CustomSchema) (*model.CustomSchema, error) {
	var id string
	var err error
	if c.ID == "" {
		err = s.DB.QueryRowContext(ctx, `INSERT INTO custom_schemas (name, preference_domain, schema, created_by) VALUES ($1,$2,$3::jsonb,$4) RETURNING id`,
			c.Name, c.PreferenceDomain, string(c.Schema), c.CreatedBy).Scan(&id)
	} else {
		id = c.ID
		var res sql.Result
		res, err = s.DB.ExecContext(ctx, `UPDATE custom_schemas SET name=$2, preference_domain=$3, schema=$4::jsonb, updated_at=now() WHERE id=$1`,
			c.ID, c.Name, c.PreferenceDomain, string(c.Schema))
		if err == nil {
			if n, _ := res.RowsAffected(); n == 0 {
				return nil, ErrNotFound
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return s.GetCustomSchema(ctx, id)
}

// DeleteCustomSchema removes a schema; profiles built from it keep their payload.
func (s *Store) DeleteCustomSchema(ctx context.Context, id string) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE config_profiles SET schema_source = '' WHERE schema_source = $1`, "custom:"+id); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM custom_schemas WHERE id = $1`, id)
	return err
}

// ProfilesUsingSchema counts profiles built from a schema source.
func (s *Store) ProfilesUsingSchema(ctx context.Context, source string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM config_profiles WHERE schema_source = $1`, source).Scan(&n)
	return n, err
}
