package store

import (
	"context"
	"strings"

	"github.com/vibemdm/vibemdm/internal/model"
)

// ---- buildings & departments ----

func (s *Store) listNamed(ctx context.Context, table, col string) ([]model.Building, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT t.id, t.name, (SELECT count(*) FROM computers c WHERE lower(c.`+col+`) = lower(t.name)), t.created_at, t.updated_at
		FROM `+table+` t ORDER BY lower(t.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Building{}
	for rows.Next() {
		var b model.Building
		if err := rows.Scan(&b.ID, &b.Name, &b.Computers, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListBuildings returns buildings with computer counts.
func (s *Store) ListBuildings(ctx context.Context) ([]model.Building, error) {
	return s.listNamed(ctx, "buildings", "building")
}

// ListDepartments returns departments with computer counts.
func (s *Store) ListDepartments(ctx context.Context) ([]model.Department, error) {
	bs, err := s.listNamed(ctx, "departments", "department")
	if err != nil {
		return nil, err
	}
	out := make([]model.Department, len(bs))
	for i, b := range bs {
		out[i] = model.Department(b)
	}
	return out, nil
}

// CreateNamed inserts a building or department.
func (s *Store) CreateNamed(ctx context.Context, table, name string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO `+table+` (name) VALUES ($1) RETURNING id`, strings.TrimSpace(name)).Scan(&id)
	return id, err
}

// RenameNamed renames a building/department and updates computers that reference the old name.
func (s *Store) RenameNamed(ctx context.Context, table, col, id, newName string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM `+table+` WHERE id = $1 FOR UPDATE`, id).Scan(&old); err != nil {
		return wrapNotFound(err)
	}
	newName = strings.TrimSpace(newName)
	if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET name = $2, updated_at = now() WHERE id = $1`, id, newName); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE computers SET `+col+` = $2 WHERE lower(`+col+`) = lower($1)`, old, newName); err != nil {
		return err
	}
	// Scopes reference buildings/departments by name; rewrite stored scopes.
	for _, scopeTable := range []string{"policies", "config_profiles"} {
		if _, err := tx.ExecContext(ctx, `UPDATE `+scopeTable+` SET scope = replace(scope::text, to_json($1::text)::text, to_json($2::text)::text)::jsonb
			WHERE scope::text LIKE '%'||to_json($1::text)::text||'%'`, old, newName); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteNamed removes a building/department (computer fields keep their text).
func (s *Store) DeleteNamed(ctx context.Context, table, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM `+table+` WHERE id = $1`, id)
	return err
}

// ---- network segments ----

const segCols = `s.id, s.name, host(s.starting_address), host(s.ending_address), s.assign_building, s.assign_department,
	(SELECT count(*) FROM computers c WHERE c.ip_address <> '' AND c.ip_address::inet BETWEEN s.starting_address AND s.ending_address), s.created_at, s.updated_at`

func scanSegment(row interface{ Scan(...any) error }) (*model.NetworkSegment, error) {
	var n model.NetworkSegment
	if err := row.Scan(&n.ID, &n.Name, &n.StartingAddress, &n.EndingAddress, &n.AssignBuilding, &n.AssignDepartment, &n.Computers, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	return &n, nil
}

// ListNetworkSegments returns segments with live computer counts.
func (s *Store) ListNetworkSegments(ctx context.Context) ([]model.NetworkSegment, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+segCols+` FROM network_segments s ORDER BY s.starting_address`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.NetworkSegment{}
	for rows.Next() {
		n, err := scanSegment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// CreateNetworkSegment inserts a segment.
func (s *Store) CreateNetworkSegment(ctx context.Context, n model.NetworkSegment) (*model.NetworkSegment, error) {
	return scanSegment(s.DB.QueryRowContext(ctx, `INSERT INTO network_segments (name, starting_address, ending_address, assign_building, assign_department)
		VALUES ($1, $2::inet, $3::inet, $4, $5) RETURNING `+strings.ReplaceAll(segCols, "s.", ""),
		strings.TrimSpace(n.Name), n.StartingAddress, n.EndingAddress, n.AssignBuilding, n.AssignDepartment))
}

// UpdateNetworkSegment updates a segment.
func (s *Store) UpdateNetworkSegment(ctx context.Context, n model.NetworkSegment) (*model.NetworkSegment, error) {
	return scanSegment(s.DB.QueryRowContext(ctx, `UPDATE network_segments SET name=$2, starting_address=$3::inet, ending_address=$4::inet,
		assign_building=$5, assign_department=$6, updated_at=now() WHERE id=$1 RETURNING `+strings.ReplaceAll(segCols, "s.", ""),
		n.ID, strings.TrimSpace(n.Name), n.StartingAddress, n.EndingAddress, n.AssignBuilding, n.AssignDepartment))
}

// DeleteNetworkSegment removes a segment.
func (s *Store) DeleteNetworkSegment(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM network_segments WHERE id = $1`, id)
	return err
}

// ComputerIDsInSegments returns IDs of computers whose last IP is inside any of the segments.
func (s *Store) ComputerIDsInSegments(ctx context.Context, segmentIDs []string) ([]string, error) {
	if len(segmentIDs) == 0 {
		return nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT c.id FROM computers c JOIN network_segments s
		ON c.ip_address <> '' AND c.ip_address::inet BETWEEN s.starting_address AND s.ending_address
		WHERE s.id::text = ANY($1)`, segmentIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// IPInSegments reports whether ip lies inside any of the given segments.
func (s *Store) IPInSegments(ctx context.Context, ip string, segmentIDs []string) (bool, error) {
	if ip == "" || len(segmentIDs) == 0 {
		return false, nil
	}
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM network_segments WHERE id::text = ANY($1)
		AND $2::inet BETWEEN starting_address AND ending_address`, segmentIDs, ip).Scan(&n)
	if err != nil {
		// invalid IP text — treat as no match
		return false, nil
	}
	return n > 0, nil
}

// ApplySegmentAssignments sets building/department on a computer from the
// segments covering its IP (Jamf-style auto-assignment). No-op without matches.
func (s *Store) ApplySegmentAssignments(ctx context.Context, computerID, ip string) {
	if ip == "" {
		return
	}
	s.DB.ExecContext(ctx, `UPDATE computers c SET
		building = COALESCE(NULLIF((SELECT n.assign_building FROM network_segments n WHERE $2::inet BETWEEN n.starting_address AND n.ending_address AND n.assign_building <> '' ORDER BY n.ending_address - n.starting_address LIMIT 1),''), c.building),
		department = COALESCE(NULLIF((SELECT n.assign_department FROM network_segments n WHERE $2::inet BETWEEN n.starting_address AND n.ending_address AND n.assign_department <> '' ORDER BY n.ending_address - n.starting_address LIMIT 1),''), c.department)
		WHERE c.id = $1`, computerID, ip)
}
