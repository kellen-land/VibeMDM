package agent

import (
	"context"
	"testing"
)

func TestNotifyMode(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "success"},
		{[]string{"firefoxpkg", "DEBUG='0'", "NOTIFY='all'"}, "all"},
		{[]string{"x", "NOTIFY='success'", "NOTIFY=silent"}, "silent"},
	} {
		if got := notifyMode(c.args); got != c.want {
			t.Errorf("notifyMode(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

const fresh = `2026-10-11 10:00:00 : INFO  : firefoxpkg : name: Firefox, appName: Firefox.app
2026-10-11 10:00:01 : INFO  : firefoxpkg : App(s) found: 
2026-10-11 10:00:40 : INFO  : firefoxpkg : Finishing...
2026-10-11 10:00:43 : INFO  : firefoxpkg : found app at /Applications/Firefox.app, version 131.0, on versionKey CFBundleShortVersionString
2026-10-11 10:00:43 : REQ   : firefoxpkg : Installed Firefox, version 131.0
2026-10-11 10:00:43 : REQ   : firefoxpkg : ################## End Installomator, exit code 0 
`

const update = `2026-10-11 10:00:01 : INFO  : firefoxpkg : found app at /Applications/Firefox.app, version 130.0, on versionKey CFBundleShortVersionString
2026-10-11 10:00:43 : INFO  : firefoxpkg : found app at /Applications/Firefox.app, version 131.0, on versionKey CFBundleShortVersionString
2026-10-11 10:00:43 : REQ   : firefoxpkg : Installed Firefox, version 131.0
`

const appStore = `2026-10-11 10:00:01 : WARN  : pages : Installed Pages.app is from App Store, use “IGNORE_APP_STORE_APPS=yes” to replace.
`

func TestAppNotification(t *testing.T) {
	e := &Executor{}
	a := appRun{Name: "Firefox", Label: "firefoxpkg"}
	n, ok := e.appNotification(context.Background(), a, "success", ExecResult{Combined: fresh})
	if !ok || n.Title != "Firefox is ready" || n.Message != "Installed Firefox, version 131.0" || n.Open != "/Applications/Firefox.app" || n.Icon != n.Open {
		t.Fatalf("fresh install: %+v %v", n, ok)
	}
	if n, _ := e.appNotification(context.Background(), a, "success", ExecResult{Combined: update}); n.Title != "Firefox was updated" {
		t.Fatalf("update: %+v", n)
	}
	if _, ok := e.appNotification(context.Background(), a, "silent", ExecResult{Combined: fresh}); ok {
		t.Fatal("silent notified")
	}
	if _, ok := e.appNotification(context.Background(), a, "success", ExecResult{Combined: appStore}); ok {
		t.Fatal("success notified without an install")
	}
	if n, ok := e.appNotification(context.Background(), a, "all", ExecResult{ExitCode: 1, Combined: "boom"}); !ok || n.Title != "Firefox could not be installed" {
		t.Fatalf("failure: %+v", n)
	}
}
