package localaccount

import (
	"strings"
	"testing"

	"github.com/vibemdm/vibemdm/internal/model"
)

func TestValidate(t *testing.T) {
	ok := []model.PolicyAccount{
		{Action: Create, Username: "student", FullName: "Student", Password: "Welcome2School"},
		{Action: Create, Username: "Lab.Admin", Password: "pw-1234", HomeDirectory: "/Users/labadmin", Admin: true},
		{Action: ResetPassword, Username: "student", Password: "NewPw-5678"},
		{Action: Delete, Username: "olduser", DeleteHome: true},
		{Action: DisableFileVault, Username: "student"},
	}
	for _, a := range ok {
		if err := Validate(a, a.Password != ""); err != nil {
			t.Errorf("%+v: %v", a, err)
		}
	}
	bad := map[string]model.PolicyAccount{
		"action":    {Action: "promote", Username: "x"},
		"root":      {Action: Delete, Username: "root"},
		"system":    {Action: Delete, Username: "_mbsetupuser"},
		"spaces":    {Action: Delete, Username: "two words"},
		"no pw":     {Action: Create, Username: "student"},
		"short pw":  {Action: ResetPassword, Username: "student", Password: "abc"},
		"control":   {Action: Create, Username: "student", Password: "abc\ndef"},
		"hint":      {Action: Create, Username: "student", Password: "Secret-99", PasswordHint: "it is secret-99"},
		"home":      {Action: Create, Username: "student", Password: "Secret-99", HomeDirectory: "Users/student"},
		"home dots": {Action: Create, Username: "student", Password: "Secret-99", HomeDirectory: "/Users/../var/root"},
	}
	for name, a := range bad {
		if err := Validate(a, a.Password != ""); err == nil {
			t.Errorf("%s: accepted %+v", name, a)
		} else if a.Password != "" && strings.Contains(err.Error(), a.Password) {
			t.Errorf("%s: error leaks the password: %v", name, err)
		}
	}
	// A stored password satisfies "required" on edit.
	if err := Validate(model.PolicyAccount{Action: Create, Username: "student"}, true); err != nil {
		t.Fatal(err)
	}
}
