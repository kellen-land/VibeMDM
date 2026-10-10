package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

// GetSuperConfig returns the super deployment settings and the (unsealed) auth password.
func (s *Store) GetSuperConfig(ctx context.Context) (*model.SuperConfig, string, error) {
	c := model.SuperConfig{AuthMode: "none"}
	var sc db.JSON[model.Scope]
	var sealed string
	err := s.DB.QueryRowContext(ctx, `SELECT enabled, scope, auth_mode, auth_account, auth_password, profile_id, updated_at FROM super_config WHERE id = 1`).
		Scan(&c.Enabled, &sc, &c.AuthMode, &c.AuthAccount, &sealed, &c.ProfileID, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		c.Scope.Normalize()
		return &c, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	c.Scope = sc.V
	c.Scope.Normalize()
	pw := ""
	if sealed != "" {
		if pw, err = s.Box.Open(sealed); err != nil {
			return nil, "", err
		}
		c.HasPassword = true
	}
	return &c, pw, nil
}

// SaveSuperConfig stores the deployment. password: nil keeps the current one, "" clears it.
func (s *Store) SaveSuperConfig(ctx context.Context, c model.SuperConfig, password *string) error {
	c.Scope.Normalize()
	if password == nil {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO super_config (id, enabled, scope, auth_mode, auth_account, profile_id) VALUES (1,$1,$2::jsonb,$3,$4,$5)
			ON CONFLICT (id) DO UPDATE SET enabled=EXCLUDED.enabled, scope=EXCLUDED.scope, auth_mode=EXCLUDED.auth_mode,
			auth_account=EXCLUDED.auth_account, profile_id=EXCLUDED.profile_id, updated_at=now()`,
			c.Enabled, jsonArg(c.Scope), c.AuthMode, c.AuthAccount, c.ProfileID)
		return err
	}
	sealed := ""
	if *password != "" {
		var err error
		if sealed, err = s.Box.Seal(*password); err != nil {
			return err
		}
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO super_config (id, enabled, scope, auth_mode, auth_account, auth_password, profile_id) VALUES (1,$1,$2::jsonb,$3,$4,$5,$6)
		ON CONFLICT (id) DO UPDATE SET enabled=EXCLUDED.enabled, scope=EXCLUDED.scope, auth_mode=EXCLUDED.auth_mode,
		auth_account=EXCLUDED.auth_account, auth_password=EXCLUDED.auth_password, profile_id=EXCLUDED.profile_id, updated_at=now()`,
		c.Enabled, jsonArg(c.Scope), c.AuthMode, c.AuthAccount, sealed, c.ProfileID)
	return err
}

// SuperStatusRow is one Mac's super state from agent inventory.
type SuperStatusRow struct {
	ComputerID   string         `json:"computer_id"`
	ComputerName string         `json:"computer_name"`
	OSVersion    string         `json:"os_version"`
	AgentOnline  bool           `json:"agent_recent"`
	Super        map[string]any `json:"super"` // nil when not installed / not reported
	LastInvAt    *string        `json:"last_inventory_at"`
}

// SuperStatus returns super state for the given computers.
func (s *Store) SuperStatus(ctx context.Context, ids []string) ([]SuperStatusRow, error) {
	out := []SuperStatusRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, os_version, COALESCE(agent_last_checkin > now() - interval '1 hour', FALSE),
		inventory->'extra'->'super', to_char(last_inventory_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM computers WHERE id = ANY($1::uuid[]) ORDER BY lower(name)`, "{"+joinQuoted(ids)+"}")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r SuperStatusRow
		var sup db.JSON[map[string]any]
		var inv sql.NullString
		if err := rows.Scan(&r.ComputerID, &r.ComputerName, &r.OSVersion, &r.AgentOnline, &sup, &inv); err != nil {
			return nil, err
		}
		r.Super = sup.V
		if inv.Valid {
			r.LastInvAt = &inv.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
