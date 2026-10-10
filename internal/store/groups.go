package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/model"
)

// CriterionField describes a smart-group field for the UI.
type CriterionField struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Type      string   `json:"type"` // string | number | bool | date | version | group | ea
	Operators []string `json:"operators"`
}

var (
	stringOps  = []string{"is", "is_not", "like", "not_like", "matches_regex", "has_value", "has_no_value"}
	numberOps  = []string{"is", "is_not", "greater_than", "less_than", "greater_or_equal", "less_or_equal"}
	boolOps    = []string{"is", "is_not"}
	dateOps    = []string{"more_than_x_days_ago", "less_than_x_days_ago", "before_date", "after_date", "has_value", "has_no_value"}
	versionOps = []string{"is", "is_not", "like", "not_like", "greater_than", "less_than", "greater_or_equal", "less_or_equal"}
	groupOps   = []string{"member_of", "not_member_of"}
)

type fieldDef struct {
	label  string
	typ    string
	expr   string // SQL expression on alias c, or "" for special handling
	exists string // template for EXISTS subqueries: %s is the comparison placeholder on column alias x
}

var fieldDefs = map[string]fieldDef{
	"computer_name":           {"Computer Name", "string", "c.name", ""},
	"hostname":                {"Hostname", "string", "c.hostname", ""},
	"serial_number":           {"Serial Number", "string", "c.serial_number", ""},
	"udid":                    {"UDID", "string", "c.udid", ""},
	"model_name":              {"Model", "string", "c.model_name", ""},
	"model_identifier":        {"Model Identifier", "string", "c.model_identifier", ""},
	"os_version":              {"Operating System Version", "version", "c.os_version", ""},
	"os_build":                {"Operating System Build", "string", "c.os_build", ""},
	"architecture":            {"Architecture", "string", "c.architecture", ""},
	"cpu":                     {"Processor", "string", "c.cpu", ""},
	"cpu_cores":               {"Processor Cores", "number", "c.cpu_cores", ""},
	"memory_mb":               {"Total RAM (MB)", "number", "c.memory_mb", ""},
	"disk_total_mb":           {"Boot Drive Size (MB)", "number", "c.disk_total_mb", ""},
	"disk_free_mb":            {"Boot Drive Available (MB)", "number", "c.disk_free_mb", ""},
	"ip_address":              {"IP Address", "string", "c.ip_address", ""},
	"mac_address":             {"MAC Address", "string", "c.mac_address", ""},
	"last_user":               {"Last User", "string", "c.last_user", ""},
	"assigned_user":           {"Assigned Username", "string", "c.assigned_user", ""},
	"assigned_email":          {"Assigned Email", "string", "c.assigned_email", ""},
	"department":              {"Department", "string", "c.department", ""},
	"building":                {"Building", "string", "c.building", ""},
	"site":                    {"Site", "string", "c.site", ""},
	"asset_tag":               {"Asset Tag", "string", "c.asset_tag", ""},
	"agent_version":           {"Agent Version", "version", "c.agent_version", ""},
	"filevault_enabled":       {"FileVault Enabled", "bool", "c.filevault_enabled", ""},
	"sip_enabled":             {"SIP Enabled", "bool", "c.sip_enabled", ""},
	"firewall_enabled":        {"Firewall Enabled", "bool", "c.firewall_enabled", ""},
	"gatekeeper_enabled":      {"Gatekeeper Enabled", "bool", "c.gatekeeper_enabled", ""},
	"is_supervised":           {"Supervised", "bool", "c.is_supervised", ""},
	"activation_lock_enabled": {"Activation Lock Enabled", "bool", "c.activation_lock_enabled", ""},
	"mdm_enrolled":            {"MDM Enrolled", "bool", "c.mdm_enrolled", ""},
	"agent_enrolled":          {"Agent Installed", "bool", "c.agent_enrolled", ""},
	"secure_boot_level":       {"Secure Boot Level", "string", "c.secure_boot_level", ""},
	"last_checkin":            {"Last Check-in", "date", "GREATEST(c.agent_last_checkin, c.mdm_last_checkin)", ""},
	"last_inventory":          {"Last Inventory Update", "date", "c.last_inventory_at", ""},
	"mdm_enrolled_at":         {"MDM Enrollment Date", "date", "c.mdm_enrolled_at", ""},
	"agent_enrolled_at":       {"Agent Enrollment Date", "date", "c.agent_enrolled_at", ""},
	"created_at":              {"Record Created", "date", "c.created_at", ""},
	"application_title":       {"Application Title", "string", "", `EXISTS (SELECT 1 FROM computer_apps x WHERE x.computer_id = c.id AND %s)`},
	"application_version":     {"Application Version", "string", "", `EXISTS (SELECT 1 FROM computer_apps x WHERE x.computer_id = c.id AND %s)`},
	"application_bundle_id":   {"Application Bundle ID", "string", "", `EXISTS (SELECT 1 FROM computer_apps x WHERE x.computer_id = c.id AND %s)`},
	"profile_identifier":      {"Profile Identifier", "string", "", `EXISTS (SELECT 1 FROM computer_profiles x WHERE x.computer_id = c.id AND %s)`},
	"profile_name":            {"Profile Name", "string", "", `EXISTS (SELECT 1 FROM computer_profiles x WHERE x.computer_id = c.id AND %s)`},
	"local_username":          {"Local User Account", "string", "", `EXISTS (SELECT 1 FROM computer_local_users x WHERE x.computer_id = c.id AND %s)`},
	"group_membership":        {"Computer Group", "group", "", ""},
}

var existsCols = map[string]string{
	"application_title": "x.name", "application_version": "x.version", "application_bundle_id": "x.bundle_id",
	"profile_identifier": "x.identifier", "profile_name": "x.display_name", "local_username": "x.username",
}

// CriterionFields lists fields for the UI, including extension attributes.
func (s *Store) CriterionFields(ctx context.Context) ([]CriterionField, error) {
	order := []string{"computer_name", "hostname", "serial_number", "udid", "model_name", "model_identifier", "os_version", "os_build", "architecture",
		"cpu", "cpu_cores", "memory_mb", "disk_total_mb", "disk_free_mb", "ip_address", "mac_address", "last_user", "assigned_user", "assigned_email",
		"department", "building", "site", "asset_tag", "agent_version", "filevault_enabled", "sip_enabled", "firewall_enabled", "gatekeeper_enabled",
		"is_supervised", "activation_lock_enabled", "mdm_enrolled", "agent_enrolled", "secure_boot_level", "last_checkin", "last_inventory",
		"mdm_enrolled_at", "agent_enrolled_at", "created_at", "application_title", "application_version", "application_bundle_id", "profile_identifier",
		"profile_name", "local_username", "group_membership"}
	out := make([]CriterionField, 0, len(order)+8)
	for _, k := range order {
		d := fieldDefs[k]
		out = append(out, CriterionField{Key: k, Label: d.label, Type: d.typ, Operators: opsFor(d.typ)})
	}
	eas, err := s.ListExtensionAttributes(ctx)
	if err != nil {
		return nil, err
	}
	for _, ea := range eas {
		typ := "string"
		switch ea.DataType {
		case "integer":
			typ = "number"
		case "date":
			typ = "date"
		}
		out = append(out, CriterionField{Key: "ea:" + ea.ID, Label: "EA: " + ea.Name, Type: typ, Operators: opsFor(typ)})
	}
	return out, nil
}

func opsFor(typ string) []string {
	switch typ {
	case "number":
		return numberOps
	case "bool":
		return boolOps
	case "date":
		return dateOps
	case "version":
		return versionOps
	case "group":
		return groupOps
	}
	return stringOps
}

// criteriaSQL converts criteria to a WHERE fragment on alias c. argOffset is the number of existing args.
func (s *Store) criteriaSQL(ctx context.Context, criteria []model.Criterion, argOffset int, depth int) (string, []any, error) {
	if depth > 6 {
		return "", nil, errors.New("smart group nesting too deep")
	}
	var sb strings.Builder
	var args []any
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", argOffset+len(args))
	}
	parens := 0
	for i, cr := range criteria {
		if i > 0 {
			op := "AND"
			if strings.EqualFold(cr.AndOr, "or") {
				op = "OR"
			}
			sb.WriteString(" " + op + " ")
		}
		if cr.OpenParen {
			sb.WriteString("(")
			parens++
		}
		expr, err := s.criterionExpr(ctx, cr, next, argOffset, &args, depth)
		if err != nil {
			return "", nil, err
		}
		sb.WriteString(expr)
		if cr.CloseParen && parens > 0 {
			sb.WriteString(")")
			parens--
		}
	}
	for parens > 0 {
		sb.WriteString(")")
		parens--
	}
	if sb.Len() == 0 {
		return "FALSE", nil, nil
	}
	return "(" + sb.String() + ")", args, nil
}

func (s *Store) criterionExpr(ctx context.Context, cr model.Criterion, next func(any) string, argOffset int, args *[]any, depth int) (string, error) {
	field := cr.Field
	var typ, expr, existsTpl string
	if strings.HasPrefix(field, "ea:") {
		eaID := strings.TrimPrefix(field, "ea:")
		ea, err := s.GetExtensionAttribute(ctx, eaID)
		if err != nil {
			return "FALSE", nil
		}
		typ = "string"
		switch ea.DataType {
		case "integer":
			typ = "number"
		case "date":
			typ = "date"
		}
		valExpr := "x.value"
		if typ == "number" {
			valExpr = "NULLIF(regexp_replace(x.value, '[^0-9.-]', '', 'g'), '')::numeric"
		} else if typ == "date" {
			valExpr = "NULLIF(x.value,'')::timestamptz"
		}
		cmp, err := compare(typ, valExpr, cr.Operator, cr.Value, next)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`EXISTS (SELECT 1 FROM computer_ea_values x WHERE x.computer_id = c.id AND x.ea_id = %s AND %s)`, next(eaID), cmp), nil
	}
	d, ok := fieldDefs[field]
	if !ok {
		return "", fmt.Errorf("unknown criterion field %q", field)
	}
	typ, expr, existsTpl = d.typ, d.expr, d.exists
	if typ == "group" {
		sub, subArgs, err := s.groupMembershipSQL(ctx, cr.Value, argOffset+len(*args), depth+1)
		if err != nil {
			return "", err
		}
		*args = append(*args, subArgs...)
		if cr.Operator == "not_member_of" {
			return "NOT " + sub, nil
		}
		return sub, nil
	}
	if existsTpl != "" {
		cmp, err := compare(typ, existsCols[field], cr.Operator, cr.Value, next)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(existsTpl, cmp), nil
	}
	return compare(typ, expr, cr.Operator, cr.Value, next)
}

func compare(typ, expr, op, value string, next func(any) string) (string, error) {
	switch typ {
	case "bool":
		b := value == "true" || value == "1" || strings.EqualFold(value, "yes")
		if op == "is_not" {
			b = !b
		}
		if b {
			return fmt.Sprintf("COALESCE(%s, FALSE) = TRUE", expr), nil
		}
		return fmt.Sprintf("COALESCE(%s, FALSE) = FALSE", expr), nil
	case "number":
		n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return "", fmt.Errorf("numeric value required for %q", op)
		}
		sym := map[string]string{"is": "=", "is_not": "<>", "greater_than": ">", "less_than": "<", "greater_or_equal": ">=", "less_or_equal": "<="}[op]
		if sym == "" {
			return "", fmt.Errorf("unsupported operator %q", op)
		}
		return fmt.Sprintf("(%s)::numeric %s %s", expr, sym, next(n)), nil
	case "date":
		switch op {
		case "more_than_x_days_ago":
			d, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return "", fmt.Errorf("days required")
			}
			return fmt.Sprintf("(%s) < now() - make_interval(days => %s::int)", expr, next(d)), nil
		case "less_than_x_days_ago":
			d, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return "", fmt.Errorf("days required")
			}
			return fmt.Sprintf("(%s) > now() - make_interval(days => %s::int)", expr, next(d)), nil
		case "before_date":
			return fmt.Sprintf("(%s) < %s::timestamptz", expr, next(value)), nil
		case "after_date":
			return fmt.Sprintf("(%s) > %s::timestamptz", expr, next(value)), nil
		case "has_value":
			return fmt.Sprintf("(%s) IS NOT NULL", expr), nil
		case "has_no_value":
			return fmt.Sprintf("(%s) IS NULL", expr), nil
		}
		return "", fmt.Errorf("unsupported operator %q", op)
	case "version":
		switch op {
		case "greater_than", "less_than", "greater_or_equal", "less_or_equal":
			sym := map[string]string{"greater_than": ">", "less_than": "<", "greater_or_equal": ">=", "less_or_equal": "<="}[op]
			return fmt.Sprintf("vibemdm_version_array(%s) %s vibemdm_version_array(%s)", expr, sym, next(value)), nil
		}
		typ = "string"
		fallthrough
	default:
		switch op {
		case "is":
			return fmt.Sprintf("lower(COALESCE(%s,'')) = lower(%s)", expr, next(value)), nil
		case "is_not":
			return fmt.Sprintf("lower(COALESCE(%s,'')) <> lower(%s)", expr, next(value)), nil
		case "like":
			return fmt.Sprintf("COALESCE(%s,'') ILIKE %s", expr, next(likeArg(value))), nil
		case "not_like":
			return fmt.Sprintf("COALESCE(%s,'') NOT ILIKE %s", expr, next(likeArg(value))), nil
		case "matches_regex":
			return fmt.Sprintf("COALESCE(%s,'') ~* %s", expr, next(value)), nil
		case "has_value":
			return fmt.Sprintf("COALESCE(%s,'') <> ''", expr), nil
		case "has_no_value":
			return fmt.Sprintf("COALESCE(%s,'') = ''", expr), nil
		}
		return "", fmt.Errorf("unsupported operator %q", op)
	}
}

// getGroupRaw fetches a group without computing its member count (used internally to avoid recursion).
func (s *Store) getGroupRaw(ctx context.Context, id string) (*model.ComputerGroup, error) {
	return scanGroup(s.DB.QueryRowContext(ctx, `SELECT `+groupCols+` FROM computer_groups g WHERE g.id::text = $1`, id))
}

// groupMembershipSQL returns an EXISTS/criteria fragment testing whether c is in the group.
func (s *Store) groupMembershipSQL(ctx context.Context, groupID string, argOffset int, depth int) (string, []any, error) {
	g, err := s.getGroupRaw(ctx, groupID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "FALSE", nil, nil
		}
		return "", nil, err
	}
	if !g.IsSmart {
		return fmt.Sprintf(`EXISTS (SELECT 1 FROM computer_group_members m WHERE m.group_id = $%d AND m.computer_id = c.id)`, argOffset+1), []any{groupID}, nil
	}
	return s.criteriaSQL(ctx, g.Criteria, argOffset, depth)
}

// ---- groups CRUD ----

const groupCols = `g.id, g.name, g.description, g.is_smart, g.criteria, g.created_at, g.updated_at`

func scanGroup(row interface{ Scan(...any) error }) (*model.ComputerGroup, error) {
	var g model.ComputerGroup
	var crit db.JSON[[]model.Criterion]
	if err := row.Scan(&g.ID, &g.Name, &g.Description, &g.IsSmart, &crit, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return nil, wrapNotFound(err)
	}
	g.Criteria = crit.V
	if g.Criteria == nil {
		g.Criteria = []model.Criterion{}
	}
	return &g, nil
}

// ListGroups returns all groups with member counts.
func (s *Store) ListGroups(ctx context.Context) ([]model.ComputerGroup, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+groupCols+` FROM computer_groups g ORDER BY lower(g.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ComputerGroup{}
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		n, err := s.CountGroupMembers(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].MemberCount = n
	}
	return out, nil
}

// GetGroup returns a group.
func (s *Store) GetGroup(ctx context.Context, id string) (*model.ComputerGroup, error) {
	g, err := scanGroup(s.DB.QueryRowContext(ctx, `SELECT `+groupCols+` FROM computer_groups g WHERE g.id = $1`, id))
	if err != nil {
		return nil, err
	}
	g.MemberCount, _ = s.CountGroupMembers(ctx, g.ID)
	return g, nil
}

// CreateGroup inserts a group.
func (s *Store) CreateGroup(ctx context.Context, g model.ComputerGroup) (*model.ComputerGroup, error) {
	if g.Criteria == nil {
		g.Criteria = []model.Criterion{}
	}
	return scanGroup(s.DB.QueryRowContext(ctx, `INSERT INTO computer_groups (name, description, is_smart, criteria) VALUES ($1,$2,$3,$4::jsonb) RETURNING `+strings.ReplaceAll(groupCols, "g.", ""),
		g.Name, g.Description, g.IsSmart, jsonArg(g.Criteria)))
}

// UpdateGroup updates a group.
func (s *Store) UpdateGroup(ctx context.Context, g model.ComputerGroup) (*model.ComputerGroup, error) {
	if g.Criteria == nil {
		g.Criteria = []model.Criterion{}
	}
	return scanGroup(s.DB.QueryRowContext(ctx, `UPDATE computer_groups SET name=$2, description=$3, is_smart=$4, criteria=$5::jsonb, updated_at=now() WHERE id=$1 RETURNING `+strings.ReplaceAll(groupCols, "g.", ""),
		g.ID, g.Name, g.Description, g.IsSmart, jsonArg(g.Criteria)))
}

// DeleteGroup removes a group.
func (s *Store) DeleteGroup(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM computer_groups WHERE id=$1`, id)
	return err
}

// SetStaticMembers replaces the static membership list.
func (s *Store) SetStaticMembers(ctx context.Context, groupID string, computerIDs []string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM computer_group_members WHERE group_id=$1`, groupID); err != nil {
		return err
	}
	for _, id := range computerIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO computer_group_members (group_id, computer_id) SELECT $1, id FROM computers WHERE id::text = $2 ON CONFLICT DO NOTHING`, groupID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddStaticMembers adds computers to a static group.
func (s *Store) AddStaticMembers(ctx context.Context, groupID string, computerIDs []string) error {
	for _, id := range computerIDs {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO computer_group_members (group_id, computer_id) SELECT $1, id FROM computers WHERE id::text = $2 ON CONFLICT DO NOTHING`, groupID, id); err != nil {
			return err
		}
	}
	return nil
}

// RemoveStaticMember removes a computer from a static group.
func (s *Store) RemoveStaticMember(ctx context.Context, groupID, computerID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM computer_group_members WHERE group_id=$1 AND computer_id=$2`, groupID, computerID)
	return err
}

// GroupMemberIDs resolves the member computer IDs of a group.
func (s *Store) GroupMemberIDs(ctx context.Context, groupID string) ([]string, error) {
	sub, args, err := s.groupMembershipSQL(ctx, groupID, 0, 0)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id FROM computers c WHERE `+sub, args...)
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

// CountGroupMembers counts members.
func (s *Store) CountGroupMembers(ctx context.Context, groupID string) (int, error) {
	sub, args, err := s.groupMembershipSQL(ctx, groupID, 0, 0)
	if err != nil {
		return 0, err
	}
	var n int
	err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM computers c WHERE `+sub, args...).Scan(&n)
	return n, err
}

// IsGroupMember tests membership.
func (s *Store) IsGroupMember(ctx context.Context, groupID, computerID string) (bool, error) {
	sub, args, err := s.groupMembershipSQL(ctx, groupID, 1, 0)
	if err != nil {
		return false, err
	}
	args = append([]any{computerID}, args...)
	var n int
	err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM computers c WHERE c.id = $1 AND `+sub, args...).Scan(&n)
	return n > 0, err
}

// PreviewCriteria evaluates criteria and returns matching computer IDs.
func (s *Store) PreviewCriteria(ctx context.Context, criteria []model.Criterion) ([]string, error) {
	sub, args, err := s.criteriaSQL(ctx, criteria, 0, 0)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id FROM computers c WHERE `+sub, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ---- scope resolution (Jamf-style: targets AND limitations AND NOT exclusions) ----

// MembershipCache memoizes group membership lookups for one computer during a request.
type MembershipCache map[string]bool

// NewMembershipCache creates an empty cache.
func NewMembershipCache() MembershipCache { return MembershipCache{} }

func lowerSet(vals []string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		if v != "" {
			m[strings.ToLower(v)] = true
		}
	}
	return m
}

// selectionIDs resolves a ScopeSelection to a set of computer IDs.
func (s *Store) selectionIDs(ctx context.Context, sel model.ScopeSelection) (map[string]bool, error) {
	set := map[string]bool{}
	for _, id := range sel.ComputerIDs {
		set[id] = true
	}
	for _, gid := range sel.GroupIDs {
		ids, err := s.GroupMemberIDs(ctx, gid)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			set[id] = true
		}
	}
	addQuery := func(query string, arg any) error {
		rows, err := s.DB.QueryContext(ctx, query, arg)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			set[id] = true
		}
		return rows.Err()
	}
	if len(sel.Buildings) > 0 {
		if err := addQuery(`SELECT id FROM computers WHERE lower(building) = ANY($1)`, lowered(sel.Buildings)); err != nil {
			return nil, err
		}
	}
	if len(sel.Departments) > 0 {
		if err := addQuery(`SELECT id FROM computers WHERE lower(department) = ANY($1)`, lowered(sel.Departments)); err != nil {
			return nil, err
		}
	}
	if len(sel.Users) > 0 {
		if err := addQuery(`SELECT id FROM computers WHERE lower(assigned_user) = ANY($1) OR lower(last_user) = ANY($1)`, lowered(sel.Users)); err != nil {
			return nil, err
		}
	}
	if len(sel.NetworkSegments) > 0 {
		ids, err := s.ComputerIDsInSegments(ctx, sel.NetworkSegments)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			set[id] = true
		}
	}
	return set, nil
}

func lowered(vals []string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, strings.ToLower(v))
	}
	return out
}

// ResolveScope returns the computer IDs an object applies to:
// targets, narrowed by limitations, minus exclusions.
func (s *Store) ResolveScope(ctx context.Context, sc model.Scope) ([]string, error) {
	sc.Normalize()
	base := map[string]bool{}
	if sc.AllComputers {
		rows, err := s.DB.QueryContext(ctx, `SELECT id FROM computers`)
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
		base, err = s.selectionIDs(ctx, sc.Targets)
		if err != nil {
			return nil, err
		}
	}
	// Limitations: intersect with each category that is set.
	intersect := func(keep map[string]bool) {
		for id := range base {
			if !keep[id] {
				delete(base, id)
			}
		}
	}
	if len(sc.Limitations.Users) > 0 {
		keep, err := s.selectionIDs(ctx, model.ScopeSelection{Users: sc.Limitations.Users})
		if err != nil {
			return nil, err
		}
		intersect(keep)
	}
	if len(sc.Limitations.NetworkSegments) > 0 {
		keep, err := s.selectionIDs(ctx, model.ScopeSelection{NetworkSegments: sc.Limitations.NetworkSegments})
		if err != nil {
			return nil, err
		}
		intersect(keep)
	}
	// Exclusions always win.
	if !sc.Exclusions.IsEmpty() {
		excl, err := s.selectionIDs(ctx, sc.Exclusions)
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

// matchSelection tests one computer against a ScopeSelection using its own
// fields (no fleet-wide queries); group membership goes through the cache.
func (s *Store) matchSelection(ctx context.Context, c *model.Computer, sel model.ScopeSelection, cache MembershipCache) (bool, error) {
	for _, id := range sel.ComputerIDs {
		if id == c.ID {
			return true, nil
		}
	}
	if len(sel.Buildings) > 0 && c.Building != "" && lowerSet(sel.Buildings)[strings.ToLower(c.Building)] {
		return true, nil
	}
	if len(sel.Departments) > 0 && c.Department != "" && lowerSet(sel.Departments)[strings.ToLower(c.Department)] {
		return true, nil
	}
	if len(sel.Users) > 0 {
		us := lowerSet(sel.Users)
		if (c.AssignedUser != "" && us[strings.ToLower(c.AssignedUser)]) || (c.LastUser != "" && us[strings.ToLower(c.LastUser)]) {
			return true, nil
		}
	}
	for _, gid := range sel.GroupIDs {
		in, ok := cache[gid]
		if !ok {
			var err error
			in, err = s.IsGroupMember(ctx, gid, c.ID)
			if err != nil {
				return false, err
			}
			cache[gid] = in
		}
		if in {
			return true, nil
		}
	}
	if len(sel.NetworkSegments) > 0 {
		in, err := s.IPInSegments(ctx, c.IPAddress, sel.NetworkSegments)
		if err != nil {
			return false, err
		}
		if in {
			return true, nil
		}
	}
	return false, nil
}

// InScopeComputer tests whether a computer is in scope, using a shared
// membership cache when evaluating many scopes for one computer.
func (s *Store) InScopeComputer(ctx context.Context, c *model.Computer, sc model.Scope, cache MembershipCache) (bool, error) {
	if cache == nil {
		cache = NewMembershipCache()
	}
	sc.Normalize()
	// Exclusions always win.
	if !sc.Exclusions.IsEmpty() {
		ex, err := s.matchSelection(ctx, c, sc.Exclusions, cache)
		if err != nil || ex {
			return false, err
		}
	}
	// Limitations: every set category must match.
	if len(sc.Limitations.Users) > 0 {
		us := lowerSet(sc.Limitations.Users)
		if !(c.AssignedUser != "" && us[strings.ToLower(c.AssignedUser)]) && !(c.LastUser != "" && us[strings.ToLower(c.LastUser)]) {
			return false, nil
		}
	}
	if len(sc.Limitations.NetworkSegments) > 0 {
		in, err := s.IPInSegments(ctx, c.IPAddress, sc.Limitations.NetworkSegments)
		if err != nil || !in {
			return false, err
		}
	}
	if sc.AllComputers {
		return true, nil
	}
	return s.matchSelection(ctx, c, sc.Targets, cache)
}

// InScope tests scope membership by computer ID (fetches the computer).
func (s *Store) InScope(ctx context.Context, computerID string, sc model.Scope) (bool, error) {
	c, err := s.GetComputer(ctx, computerID)
	if err != nil {
		return false, err
	}
	return s.InScopeComputer(ctx, c, sc, nil)
}

// InScopeCached is kept for callers that only hold an ID.
func (s *Store) InScopeCached(ctx context.Context, computerID string, sc model.Scope, cache MembershipCache) (bool, error) {
	c, err := s.GetComputer(ctx, computerID)
	if err != nil {
		return false, err
	}
	return s.InScopeComputer(ctx, c, sc, cache)
}
