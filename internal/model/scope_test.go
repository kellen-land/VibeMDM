package model

import (
	"encoding/json"
	"testing"
)

func TestScopeLegacyUnmarshal(t *testing.T) {
	legacy := []byte(`{"all_computers":false,"computer_ids":["a"],"group_ids":["g1","g2"],"excluded_computer_ids":["x"],"excluded_group_ids":["xg"]}`)
	var sc Scope
	if err := json.Unmarshal(legacy, &sc); err != nil {
		t.Fatal(err)
	}
	if len(sc.Targets.ComputerIDs) != 1 || sc.Targets.ComputerIDs[0] != "a" {
		t.Errorf("targets computers = %v", sc.Targets.ComputerIDs)
	}
	if len(sc.Targets.GroupIDs) != 2 || len(sc.Exclusions.ComputerIDs) != 1 || len(sc.Exclusions.GroupIDs) != 1 {
		t.Errorf("legacy mapping wrong: %+v", sc)
	}
	// Round-trips in the new shape.
	b, _ := json.Marshal(sc)
	var sc2 Scope
	if err := json.Unmarshal(b, &sc2); err != nil {
		t.Fatal(err)
	}
	if len(sc2.Targets.GroupIDs) != 2 || len(sc2.Exclusions.ComputerIDs) != 1 {
		t.Errorf("round trip lost data: %s", b)
	}
}

func TestScopeNewShapeUnmarshal(t *testing.T) {
	nested := []byte(`{"all_computers":true,"targets":{"buildings":["HQ"]},"limitations":{"users":["alice"],"network_segments":["s1"]},"exclusions":{"departments":["Design"]}}`)
	var sc Scope
	if err := json.Unmarshal(nested, &sc); err != nil {
		t.Fatal(err)
	}
	if !sc.AllComputers || sc.Targets.Buildings[0] != "HQ" || sc.Limitations.Users[0] != "alice" || sc.Exclusions.Departments[0] != "Design" {
		t.Errorf("parsed: %+v", sc)
	}
	if sc.Limitations.IsEmpty() || sc.Exclusions.IsEmpty() {
		t.Error("IsEmpty wrong")
	}
}
