package agent

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const daemonPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array><string>%s</string><string>daemon</string></array>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>ThrottleInterval</key><integer>15</integer>
	<key>StandardOutPath</key><string>%s</string>
	<key>StandardErrorPath</key><string>%s</string>
	<key>EnvironmentVariables</key>
	<dict><key>PATH</key><string>/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin</string></dict>
</dict>
</plist>
`

const loginPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array><string>%s</string><string>trigger</string><string>login</string></array>
	<key>RunAtLoad</key><true/>
	<key>LimitLoadToSessionType</key><string>Aqua</string>
</dict>
</plist>
`

// InstallLaunchd writes and loads the LaunchDaemon and login LaunchAgent.
func InstallLaunchd(binary string) error {
	if binary == "" {
		binary = BinaryPath
	}
	// launchd does not create directories for StandardOutPath.
	os.MkdirAll(LogDir, 0o750)
	os.Lchown(LogDir, 0, adminGID)
	if err := os.WriteFile(DaemonPlist, []byte(fmt.Sprintf(daemonPlistTemplate, DaemonLabel, binary, LaunchdLogPath, LaunchdLogPath)), 0o644); err != nil {
		return err
	}
	os.MkdirAll("/Library/LaunchAgents", 0o755)
	if err := os.WriteFile(LoginPlist, []byte(fmt.Sprintf(loginPlistTemplate, LoginLabel, binary)), 0o644); err != nil {
		return err
	}
	// (Re)load the daemon.
	exec.Command("launchctl", "bootout", "system/"+DaemonLabel).Run()
	out, err := exec.Command("launchctl", "bootstrap", "system", DaemonPlist).CombinedOutput()
	if err != nil {
		// Fall back to legacy load for older systems.
		if out2, err2 := exec.Command("launchctl", "load", "-w", DaemonPlist).CombinedOutput(); err2 != nil {
			return fmt.Errorf("launchctl bootstrap failed: %s / %s", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}
	return nil
}

// RemoveLaunchd unloads and deletes launchd jobs.
func RemoveLaunchd() {
	exec.Command("launchctl", "bootout", "system/"+DaemonLabel).Run()
	os.Remove(DaemonPlist)
	os.Remove(LoginPlist)
}

// RestartDaemon kicks the daemon so it picks up new config.
func RestartDaemon() {
	exec.Command("launchctl", "kickstart", "-k", "system/"+DaemonLabel).Run()
}
