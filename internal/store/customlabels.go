package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/installomator"
	"github.com/vibemdm/vibemdm/internal/model"
)

const labelCols = `id, key, name, type, download_url, expected_team_id, package_id, app_name, archive_name, version_key, app_new_version, blocking_processes, description, created_at, updated_at`

func scanLabel(row interface{ Scan(...any) error }) (*model.CustomLabel, error) {
	var l model.CustomLabel
	var bp db.JSON[[]string]
	if err := row.Scan(&l.ID, &l.Key, &l.Name, &l.Type, &l.DownloadURL, &l.ExpectedTeamID, &l.PackageID, &l.AppName, &l.ArchiveName, &l.VersionKey, &l.AppNewVersion, &bp, &l.Description, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	l.BlockingProcesses = bp.V
	if l.BlockingProcesses == nil {
		l.BlockingProcesses = []string{}
	}
	return &l, nil
}

// ToInstallomator converts a stored custom label for argument building.
func toInstallomator(l *model.CustomLabel) installomator.CustomLabel {
	return installomator.CustomLabel{Key: l.Key, Name: l.Name, Type: l.Type, DownloadURL: l.DownloadURL, ExpectedTeamID: l.ExpectedTeamID,
		PackageID: l.PackageID, AppName: l.AppName, ArchiveName: l.ArchiveName, VersionKey: l.VersionKey, AppNewVersion: l.AppNewVersion,
		BlockingProcesses: l.BlockingProcesses}
}

// ValidateCustomLabel checks a label definition.
func ValidateCustomLabel(l model.CustomLabel) error { return toInstallomator(&l).Validate() }

// ListCustomLabels returns custom labels.
func (s *Store) ListCustomLabels(ctx context.Context) ([]model.CustomLabel, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+labelCols+` FROM custom_labels ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CustomLabel{}
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// GetCustomLabel returns a label by ID or key.
func (s *Store) GetCustomLabel(ctx context.Context, idOrKey string) (*model.CustomLabel, error) {
	return scanLabel(s.DB.QueryRowContext(ctx, `SELECT `+labelCols+` FROM custom_labels WHERE id::text = $1 OR key = $1`, idOrKey))
}

// SaveCustomLabel creates (empty ID) or updates a label after validation.
func (s *Store) SaveCustomLabel(ctx context.Context, l model.CustomLabel) (*model.CustomLabel, error) {
	if l.BlockingProcesses == nil {
		l.BlockingProcesses = []string{}
	}
	if err := ValidateCustomLabel(l); err != nil {
		return nil, err
	}
	if l.ID == "" {
		return scanLabel(s.DB.QueryRowContext(ctx, `INSERT INTO custom_labels (key, name, type, download_url, expected_team_id, package_id, app_name, archive_name, version_key, app_new_version, blocking_processes, description)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12) RETURNING `+labelCols,
			l.Key, l.Name, l.Type, l.DownloadURL, l.ExpectedTeamID, l.PackageID, l.AppName, l.ArchiveName, l.VersionKey, l.AppNewVersion, jsonArg(l.BlockingProcesses), l.Description))
	}
	return scanLabel(s.DB.QueryRowContext(ctx, `UPDATE custom_labels SET key=$2, name=$3, type=$4, download_url=$5, expected_team_id=$6, package_id=$7, app_name=$8,
		archive_name=$9, version_key=$10, app_new_version=$11, blocking_processes=$12::jsonb, description=$13, updated_at=now() WHERE id=$1 RETURNING `+labelCols,
		l.ID, l.Key, l.Name, l.Type, l.DownloadURL, l.ExpectedTeamID, l.PackageID, l.AppName, l.ArchiveName, l.VersionKey, l.AppNewVersion, jsonArg(l.BlockingProcesses), l.Description))
}

// DeleteCustomLabel removes a label.
func (s *Store) DeleteCustomLabel(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM custom_labels WHERE id = $1`, id)
	return err
}

// InstallomatorArgs resolves a label ("custom:<key>" or a shipped label) and
// options into safe Installomator arguments plus a display name.
func (s *Store) InstallomatorArgs(ctx context.Context, label string, o model.InstallomatorOptions) ([]string, string, error) {
	opts := installomator.Options{Notify: o.Notify, BlockingProcessAction: o.BlockingProcessAction, Reopen: o.Reopen, IgnoreAppStoreApps: o.IgnoreAppStoreApps, ForceInstall: o.ForceInstall}
	if key, ok := strings.CutPrefix(label, "custom:"); ok {
		l, err := s.GetCustomLabel(ctx, key)
		if err != nil {
			return nil, "", fmt.Errorf("custom label %q: %w", key, err)
		}
		args, err := installomator.CustomArgs(toInstallomator(l), opts)
		return args, l.Name, err
	}
	args, err := installomator.BuiltinArgs(label, opts)
	return args, label, err
}
