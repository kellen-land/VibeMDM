package store

import (
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	var args []any
	next := func(v any) string { args = append(args, v); return "$1" }
	cases := []struct{ typ, op, val, want string }{
		{"string", "is", "x", "lower(COALESCE(c.name,'')) = lower($1)"},
		{"string", "like", "mac", "COALESCE(c.name,'') ILIKE $1"},
		{"bool", "is", "true", "COALESCE(c.name, FALSE) = TRUE"},
		{"number", "greater_than", "8", "(c.name)::numeric > $1"},
		{"version", "greater_or_equal", "14.0", "vibemdm_version_array(c.name) >= vibemdm_version_array($1)"},
		{"date", "more_than_x_days_ago", "30", "(c.name) < now() - make_interval(days => $1::int)"},
	}
	for _, c := range cases {
		got, err := compare(c.typ, "c.name", c.op, c.val, next)
		if err != nil {
			t.Errorf("%v: %v", c, err)
			continue
		}
		if got != c.want {
			t.Errorf("compare(%s,%s,%s) = %q, want %q", c.typ, c.op, c.val, got, c.want)
		}
	}
	if _, err := compare("number", "c.x", "is", "abc", next); err == nil {
		t.Error("expected error for non-numeric value")
	}
	if _, err := compare("string", "c.x", "bogus", "a", next); err == nil {
		t.Error("expected error for unknown operator")
	}
}

func TestLikeArgEscapes(t *testing.T) {
	if got := likeArg("50%_x"); got != `%50\%\_x%` {
		t.Errorf("likeArg = %q", got)
	}
}

func TestCriterionFieldsCoverOperators(t *testing.T) {
	for k, d := range fieldDefs {
		if len(opsFor(d.typ)) == 0 {
			t.Errorf("field %s has no operators", k)
		}
		if d.typ != "group" && d.expr == "" && d.exists == "" {
			t.Errorf("field %s has no SQL", k)
		}
		if d.exists != "" && !strings.Contains(d.exists, "%s") {
			t.Errorf("field %s exists template lacks placeholder", k)
		}
	}
}
