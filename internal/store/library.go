package store

import (
	"context"
	"strings"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

// ---- scripts ----

const scriptCols = `id, name, category, info, notes, priority, parameter_labels, contents, os_requirements, created_at, updated_at`

func scanScript(row interface{ Scan(...any) error }) (*model.Script, error) {
	var sc model.Script
	var labels db.JSON[[]string]
	if err := row.Scan(&sc.ID, &sc.Name, &sc.Category, &sc.Info, &sc.Notes, &sc.Priority, &labels, &sc.Contents, &sc.OSRequirements, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	sc.ParameterLabels = labels.V
	for len(sc.ParameterLabels) < 8 {
		sc.ParameterLabels = append(sc.ParameterLabels, "")
	}
	return &sc, nil
}

// ListScripts returns all scripts (contents omitted unless withContents).
func (s *Store) ListScripts(ctx context.Context, withContents bool) ([]model.Script, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+scriptCols+` FROM scripts ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Script{}
	for rows.Next() {
		sc, err := scanScript(rows)
		if err != nil {
			return nil, err
		}
		if !withContents {
			sc.Contents = ""
		}
		out = append(out, *sc)
	}
	return out, rows.Err()
}

// GetScript returns a script.
func (s *Store) GetScript(ctx context.Context, id string) (*model.Script, error) {
	return scanScript(s.DB.QueryRowContext(ctx, `SELECT `+scriptCols+` FROM scripts WHERE id = $1`, id))
}

func normLabels(l []string) []string {
	out := make([]string, 8)
	copy(out, l)
	return out
}

// CreateScript inserts a script.
func (s *Store) CreateScript(ctx context.Context, sc model.Script) (*model.Script, error) {
	if sc.Priority != "before" {
		sc.Priority = "after"
	}
	return scanScript(s.DB.QueryRowContext(ctx, `INSERT INTO scripts (name, category, info, notes, priority, parameter_labels, contents, os_requirements)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8) RETURNING `+scriptCols,
		sc.Name, sc.Category, sc.Info, sc.Notes, sc.Priority, jsonArg(normLabels(sc.ParameterLabels)), sc.Contents, sc.OSRequirements))
}

// UpdateScript updates a script.
func (s *Store) UpdateScript(ctx context.Context, sc model.Script) (*model.Script, error) {
	if sc.Priority != "before" {
		sc.Priority = "after"
	}
	return scanScript(s.DB.QueryRowContext(ctx, `UPDATE scripts SET name=$2, category=$3, info=$4, notes=$5, priority=$6, parameter_labels=$7::jsonb, contents=$8, os_requirements=$9, updated_at=now()
		WHERE id=$1 RETURNING `+scriptCols,
		sc.ID, sc.Name, sc.Category, sc.Info, sc.Notes, sc.Priority, jsonArg(normLabels(sc.ParameterLabels)), sc.Contents, sc.OSRequirements))
}

// DeleteScript removes a script.
func (s *Store) DeleteScript(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM scripts WHERE id=$1`, id)
	return err
}

// ---- packages ----

const packageCols = `id, name, filename, category, info, notes, size_bytes, sha256, priority, created_at, updated_at`

func scanPackage(row interface{ Scan(...any) error }) (*model.Package, error) {
	var p model.Package
	if err := row.Scan(&p.ID, &p.Name, &p.Filename, &p.Category, &p.Info, &p.Notes, &p.SizeBytes, &p.SHA256, &p.Priority, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	return &p, nil
}

// ListPackages returns packages.
func (s *Store) ListPackages(ctx context.Context) ([]model.Package, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+packageCols+` FROM packages ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Package{}
	for rows.Next() {
		p, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPackage returns a package.
func (s *Store) GetPackage(ctx context.Context, id string) (*model.Package, error) {
	return scanPackage(s.DB.QueryRowContext(ctx, `SELECT `+packageCols+` FROM packages WHERE id = $1`, id))
}

// CreatePackage inserts a package record.
func (s *Store) CreatePackage(ctx context.Context, p model.Package) (*model.Package, error) {
	if p.Priority == 0 {
		p.Priority = 10
	}
	return scanPackage(s.DB.QueryRowContext(ctx, `INSERT INTO packages (name, filename, category, info, notes, size_bytes, sha256, priority) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+packageCols,
		p.Name, p.Filename, p.Category, p.Info, p.Notes, p.SizeBytes, p.SHA256, p.Priority))
}

// UpdatePackage updates metadata.
func (s *Store) UpdatePackage(ctx context.Context, p model.Package) (*model.Package, error) {
	return scanPackage(s.DB.QueryRowContext(ctx, `UPDATE packages SET name=$2, category=$3, info=$4, notes=$5, priority=$6, updated_at=now() WHERE id=$1 RETURNING `+packageCols,
		p.ID, p.Name, p.Category, p.Info, p.Notes, p.Priority))
}

// DeletePackage removes a package record.
func (s *Store) DeletePackage(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM packages WHERE id=$1`, id)
	return err
}

// ---- extension attributes ----

const eaCols = `id, name, description, data_type, input_type, script, enabled, created_at, updated_at`

func scanEA(row interface{ Scan(...any) error }) (*model.ExtensionAttribute, error) {
	var e model.ExtensionAttribute
	if err := row.Scan(&e.ID, &e.Name, &e.Description, &e.DataType, &e.InputType, &e.Script, &e.Enabled, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	return &e, nil
}

// ListExtensionAttributes returns EAs.
func (s *Store) ListExtensionAttributes(ctx context.Context) ([]model.ExtensionAttribute, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+eaCols+` FROM extension_attributes ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ExtensionAttribute{}
	for rows.Next() {
		e, err := scanEA(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// GetExtensionAttribute returns an EA.
func (s *Store) GetExtensionAttribute(ctx context.Context, id string) (*model.ExtensionAttribute, error) {
	return scanEA(s.DB.QueryRowContext(ctx, `SELECT `+eaCols+` FROM extension_attributes WHERE id::text = $1`, id))
}

func normEA(e *model.ExtensionAttribute) {
	if e.DataType != "integer" && e.DataType != "date" {
		e.DataType = "string"
	}
	if e.InputType != "text" {
		e.InputType = "script"
	}
}

// CreateExtensionAttribute inserts an EA.
func (s *Store) CreateExtensionAttribute(ctx context.Context, e model.ExtensionAttribute) (*model.ExtensionAttribute, error) {
	normEA(&e)
	return scanEA(s.DB.QueryRowContext(ctx, `INSERT INTO extension_attributes (name, description, data_type, input_type, script, enabled) VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+eaCols,
		e.Name, e.Description, e.DataType, e.InputType, e.Script, e.Enabled))
}

// UpdateExtensionAttribute updates an EA.
func (s *Store) UpdateExtensionAttribute(ctx context.Context, e model.ExtensionAttribute) (*model.ExtensionAttribute, error) {
	normEA(&e)
	return scanEA(s.DB.QueryRowContext(ctx, `UPDATE extension_attributes SET name=$2, description=$3, data_type=$4, input_type=$5, script=$6, enabled=$7, updated_at=now() WHERE id=$1 RETURNING `+eaCols,
		e.ID, e.Name, e.Description, e.DataType, e.InputType, e.Script, e.Enabled))
}

// DeleteExtensionAttribute removes an EA.
func (s *Store) DeleteExtensionAttribute(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM extension_attributes WHERE id=$1`, id)
	return err
}

// SetEAValue sets a text-type EA value for a computer.
func (s *Store) SetEAValue(ctx context.Context, computerID, eaID, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO computer_ea_values (computer_id, ea_id, value) VALUES ($1,$2,$3)
		ON CONFLICT (computer_id, ea_id) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, computerID, eaID, value)
	return err
}

var _ = strings.TrimSpace
