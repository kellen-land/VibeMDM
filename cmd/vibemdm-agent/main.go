// Command vibemdm-agent is the VibeMDM management agent for macOS.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vibemdm/vibemdm/internal/agent"
	"github.com/vibemdm/vibemdm/internal/model"
)

var version = "dev"

const usage = `VibeMDM agent %s — management agent for macOS

Usage: vibemdm-agent <command> [options]
       vibemdm-agent help <command>     detailed help for one command

Setup
  enroll            Enroll this Mac with a VibeMDM server and install the launchd services
  manage            (Re)install the launchd daemon and login agent
  remove-framework  Unenroll and remove the agent from this Mac

Everyday
  status            Show enrollment, connectivity and local component status
  logs              Show or follow the agent log; change the log level
  policy            Check in and run policies for a trigger now
  trigger <event>   Ask the running daemon to fire a trigger
  recon             Collect inventory and submit it to the server
  checkin           Perform one check-in cycle
  check-connection  Verify the server is reachable with this Mac's credentials
  selfservice       Open the Self Service portal in the default browser

Internal
  daemon            Run the agent daemon (started by launchd)
  version           Print the agent version

Most commands need root: run them with sudo.
`

// commandHelp is the detailed help for "vibemdm-agent help <command>".
var commandHelp = map[string]string{
	"enroll": `vibemdm-agent enroll --server URL [--token TOKEN] [--mdm] [--allow-insecure-http]
vibemdm-agent enroll --from-mdm [--wait 10m]

Enrolls this Mac with a VibeMDM server, saves the device credentials to
/Library/Application Support/VibeMDM/agent.json (mode 0600) and installs the
launchd daemon and login agent. Run as root.

  --server URL            VibeMDM server URL, e.g. https://mdm.example.com (must be https)
  --token TOKEN           Enrollment token (an invitation or the enrollment secret).
                          Prefer the VIBEMDM_ENROLL_TOKEN environment variable:
                          command-line arguments are visible to other users in ps.
  --mdm                   Also download the MDM enrollment profile to /Users/Shared
                          and open it for the logged-in user to approve.
  --allow-insecure-http   Accept a plain http:// server (isolated labs only).

Re-enrolling a Mac that already has an active agent works when run on the same
Mac (the existing credentials prove it); otherwise an administrator must click
"Reset agent" on the computer in the console first.

  --from-mdm              Read the server URL and a one-time token from the profile
                          VibeMDM installs through MDM (used by the agent package's
                          postinstall when the agent is installed automatically).
  --wait DURATION         With --from-mdm, how long to wait for that profile (10m).

Example:
  sudo VIBEMDM_ENROLL_TOKEN=… vibemdm-agent enroll --server https://mdm.example.com --mdm`,
	"status": `vibemdm-agent status

Prints the server URL, computer ID, agent version, whether the daemon is
running, whether the server is reachable with this Mac's credentials, the
log level and log file, whether unified logging is built in, and the state of
local components (swiftDialog, super, remote shell opt-out). Safe to run
without sudo, but some checks need root.`,
	"logs": `vibemdm-agent logs [-n N] [-f] [--level LEVEL] [--component NAME] [--grep TEXT]
vibemdm-agent logs --unified [-f] [--last DURATION]
vibemdm-agent logs --set-level LEVEL

Shows the last N lines (default 50) of the agent log,
/Library/Logs/VibeMDM/vibemdm-agent.log (rotated at 10 MB, five old copies
kept as .1 to .5). The same file appears in Console.app under Log Reports.
Members of the admin group can read it without sudo.

  -n N                Number of lines to show (default 50).
  -f                  Keep printing new lines as they are written (Ctrl-C to stop).
  --level LEVEL       Only show LEVEL and above: debug, info, warn or error.
  --component NAME    Only show one component: daemon, checkin, policy, task,
                      package, apps, inventory, compliance, onboarding, shell,
                      restricted, selfservice, spool.
  --grep TEXT         Only show lines containing TEXT (case-insensitive).
  --unified           Read macOS unified logging instead of the file, through
                      log show (or log stream with -f), subsystem com.vibemdm.agent.
                      Needs an agent built with unified logging (see status).
  --last DURATION     With --unified: how far back to read (default 1h).
  --set-level LEVEL   Change the log level (debug, info, warn, error). Saved in
                      /Library/Application Support/VibeMDM/logging.json and applied
                      to the running daemon immediately. Run as root. Debug
                      records stay on the Mac; the server receives info and above.

Examples:
  vibemdm-agent logs -n 200 --component policy
  vibemdm-agent logs -f --level warn
  sudo vibemdm-agent logs --set-level debug`,
	"policy": `vibemdm-agent policy [--event NAME]

Checks in with the server and runs every policy scoped to this Mac for the
trigger NAME (default: checkin). Built-in triggers: checkin, startup, login,
network_change, enrollment_complete. Any other name is a custom trigger.
Run as root.

Example:
  sudo vibemdm-agent policy --event install_zoom`,
	"trigger": `vibemdm-agent trigger <event>

Asks the running daemon to fire a trigger. Any user may fire the built-in
login, checkin and network_change triggers; custom triggers require root
(sudo uses the daemon's root-only socket). If the daemon is not running and
you are root, the trigger runs directly.`,
	"recon": `vibemdm-agent recon

Collects full inventory (hardware, security, apps, profiles, users, extension
attributes, super status) and submits it. Run as root.`,
	"checkin": `vibemdm-agent checkin

Performs one check-in: reports status, runs due policies and app installs.
Run as root.`,
	"check-connection": `vibemdm-agent check-connection

Calls the server with this Mac's device token and prints its version.`,
	"selfservice": `vibemdm-agent selfservice

Opens the Self Service app for the logged-in user (or the local portal,
http://127.0.0.1:8571, in the browser when the app is not installed).`,
	"manage": `vibemdm-agent manage

Reinstalls the launchd daemon (com.vibemdm.agent) and the login agent.
Use after copying a new agent binary into place. Run as root.`,
	"remove-framework": `vibemdm-agent remove-framework

Unenrolls from the server, removes the launchd services and deletes
/Library/Application Support/VibeMDM. The MDM profile, if installed, must be
removed separately (System Settings → Device Management). Run as root.`,
	"daemon": `vibemdm-agent daemon

Runs the long-lived agent: periodic check-ins, instant tasks via long-poll,
log shipping, restricted software, Self Service portal and remote shell.
Started by launchd; you normally never run it by hand.`,
	"version": `vibemdm-agent version

Prints the agent version.`,
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd, args := os.Args[1], os.Args[2:]
	// Commands run by hand print their log to the terminal and, when root, also
	// append it to the agent log file and unified logging, so a manual
	// "sudo vibemdm-agent policy" leaves the same trail as the daemon.
	level := new(slog.LevelVar)
	level.Set(agent.LoadLogLevel(""))
	logOpts := agent.LogOptions{Level: level, Stdout: os.Stdout, Unified: true}
	if os.Geteuid() == 0 {
		logOpts.File = agent.LogPath
	}
	logger := slog.New(agent.NewLocalHandler(logOpts))
	var err error
	switch cmd {
	case "version", "-v", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		if len(args) > 0 {
			if h, ok := commandHelp[args[0]]; ok {
				fmt.Println(h)
				break
			}
			fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		}
		fmt.Printf(usage, version)
	case "status":
		err = cmdStatus(ctx)
	case "logs":
		err = cmdLogs(ctx, args)
	case "enroll":
		err = cmdEnroll(ctx, args, logger)
	case "daemon":
		err = requireRoot(func() error {
			cfg, err := agent.LoadConfig("")
			if err != nil {
				return err
			}
			// The daemon owns the log file (and rotates it), writes to unified
			// logging, and ships info and above to the server. Its stdout and
			// stderr (launchd) only catch crashes.
			local := agent.NewLocalHandler(agent.LogOptions{Level: level, File: agent.LogPath, Rotate: true, Unified: true})
			ship := agent.NewLogShipper(agent.NewClient(cfg.ServerURL, cfg.DeviceToken, version), slog.LevelInfo)
			dlog := slog.New(ship.Wrap(local))
			d := agent.NewDaemon(cfg, version, dlog)
			d.Ship = ship
			d.LogLevel = level
			return d.Run(ctx)
		})
	case "policy":
		fs := flag.NewFlagSet("policy", flag.ExitOnError)
		event := fs.String("event", "checkin", "trigger to fire")
		fs.Parse(args)
		err = requireRoot(func() error {
			cfg, err := agent.LoadConfig("")
			if err != nil {
				return err
			}
			ex := agent.NewExecutor(agent.NewClient(cfg.ServerURL, cfg.DeviceToken, version), cfg, version, logger)
			n, err := ex.CheckinAndRun(ctx, *event)
			if err == nil {
				fmt.Printf("Executed %d policies for trigger %q\n", n, *event)
			}
			return err
		})
	case "checkin":
		err = requireRoot(func() error {
			cfg, err := agent.LoadConfig("")
			if err != nil {
				return err
			}
			ex := agent.NewExecutor(agent.NewClient(cfg.ServerURL, cfg.DeviceToken, version), cfg, version, logger)
			_, err = ex.CheckinAndRun(ctx, "checkin")
			return err
		})
	case "trigger":
		if len(args) < 1 {
			err = fmt.Errorf("usage: vibemdm-agent trigger <event>")
			break
		}
		user := os.Getenv("USER")
		if os.Geteuid() == 0 {
			// Root can run directly if the daemon is down.
			if e := agent.SendLocalTrigger(args[0], user); e != nil {
				cfg, cerr := agent.LoadConfig("")
				if cerr != nil {
					err = cerr
					break
				}
				ex := agent.NewExecutor(agent.NewClient(cfg.ServerURL, cfg.DeviceToken, version), cfg, version, logger)
				_, err = ex.CheckinAndRun(ctx, args[0])
			}
		} else {
			err = agent.SendLocalTrigger(args[0], user)
		}
	case "recon":
		err = requireRoot(func() error {
			cfg, err := agent.LoadConfig("")
			if err != nil {
				return err
			}
			ex := agent.NewExecutor(agent.NewClient(cfg.ServerURL, cfg.DeviceToken, version), cfg, version, logger)
			if err := ex.Recon(ctx); err != nil {
				return err
			}
			fmt.Println("Inventory submitted.")
			return nil
		})
	case "check-connection":
		cfg, cerr := agent.LoadConfig("")
		if cerr != nil {
			err = cerr
			break
		}
		v, verr := agent.NewClient(cfg.ServerURL, cfg.DeviceToken, version).ServerVersion(ctx)
		if verr != nil {
			err = verr
			break
		}
		fmt.Printf("Connected to %s (server version %s)\n", cfg.ServerURL, v)
	case "selfservice":
		// The Self Service app when it is installed, else the portal in the browser.
		user := agent.ConsoleUser(ctx)
		url := "http://" + agent.SelfServicePort
		open := func(args ...string) error {
			if user != "" && os.Geteuid() == 0 {
				uid := agent.ConsoleUserUID(ctx, user)
				return exec.Command("launchctl", append([]string{"asuser", fmt.Sprint(uid), "sudo", "-u", user, "open"}, args...)...).Run()
			}
			return exec.Command("open", args...).Run()
		}
		if err = open("-b", "com.vibemdm.selfservice"); err == nil {
			fmt.Println("Opened the Self Service app")
		} else if err = open(url); err == nil {
			fmt.Println("Opened", url)
		}
	case "manage":
		err = requireRoot(func() error {
			if _, err := agent.LoadConfig(""); err != nil {
				return err
			}
			return agent.InstallLaunchd(currentBinary())
		})
	case "remove-framework":
		err = requireRoot(func() error {
			if cfg, err := agent.LoadConfig(""); err == nil {
				agent.NewClient(cfg.ServerURL, cfg.DeviceToken, version).Unenroll(ctx)
			}
			agent.RemoveLaunchd()
			os.Remove(agent.SocketPath)
			os.RemoveAll(agent.SupportDir)
			fmt.Println("VibeMDM agent removed. The MDM profile (if any) must be removed separately.")
			return nil
		})
	default:
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdStatus(ctx context.Context) error {
	yesno := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	fmt.Printf("Agent version:     %s\n", version)
	cfg, err := agent.LoadConfig("")
	if err != nil {
		fmt.Printf("Enrolled:          no (%v)\n", err)
		return nil
	}
	fmt.Printf("Enrolled:          yes (since %s)\n", cfg.EnrolledAt)
	fmt.Printf("Server:            %s\n", cfg.ServerURL)
	fmt.Printf("Computer ID:       %s\n", cfg.ComputerID)
	fmt.Printf("Organization:      %s\n", cfg.OrgName)
	fmt.Printf("Check-in interval: %ds\n", cfg.CheckinIntervalSeconds)
	fmt.Printf("Daemon running:    %s\n", yesno(exists(agent.SocketPath)))
	fmt.Printf("Log level:         %s\n", agent.LevelName(agent.LoadLogLevel("")))
	if fi, err := os.Stat(agent.LogPath); err == nil {
		fmt.Printf("Log file:          %s (%d KB)\n", agent.LogPath, fi.Size()>>10)
	} else {
		fmt.Printf("Log file:          %s (%v)\n", agent.LogPath, err)
	}
	if agent.UnifiedLoggingAvailable() {
		fmt.Printf("Unified logging:   yes (subsystem %s)\n", agent.LogSubsystem)
	} else {
		fmt.Printf("Unified logging:   no (this agent was built without cgo; file and server logs only)\n")
	}
	if v, err := agent.NewClient(cfg.ServerURL, cfg.DeviceToken, version).ServerVersion(ctx); err != nil {
		fmt.Printf("Server reachable:  no (%v)\n", err)
	} else {
		fmt.Printf("Server reachable:  yes (server %s)\n", v)
	}
	fmt.Printf("swiftDialog:       %s\n", yesno(exists("/usr/local/bin/dialog")))
	fmt.Printf("super:             %s\n", yesno(exists("/usr/local/bin/super")))
	fmt.Printf("Remote shell veto: %s\n", yesno(exists(agent.ShellOptOutFile)))
	fmt.Printf("Self Service:      http://%s\n", agent.SelfServicePort)
	return nil
}

func requireRoot(fn func() error) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}
	return fn()
}

func currentBinary() string {
	p, err := os.Executable()
	if err != nil {
		return agent.BinaryPath
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func cmdEnroll(ctx context.Context, args []string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	server := fs.String("server", "", "server URL")
	token := fs.String("token", "", "enrollment token (prefer the VIBEMDM_ENROLL_TOKEN environment variable: arguments are visible in ps)")
	mdm := fs.Bool("mdm", false, "also open the MDM enrollment profile")
	insecure := fs.Bool("allow-insecure-http", false, "allow a plain-http server URL (lab use only: commands would travel unencrypted)")
	fromMDM := fs.Bool("from-mdm", false, "take the server URL and a one-time token from the MDM-installed bootstrap profile")
	wait := fs.Duration("wait", 10*time.Minute, "with --from-mdm, how long to wait for the bootstrap profile")
	fs.Parse(args)
	if *fromMDM {
		b, err := agent.WaitForMDMBootstrap(ctx, *wait, nil)
		if err != nil {
			return err
		}
		*server, *token = b.ServerURL, b.EnrollToken
	}
	if *token == "" {
		*token = os.Getenv("VIBEMDM_ENROLL_TOKEN")
	}
	if *server == "" || *token == "" {
		return fmt.Errorf("--server and an enrollment token (--token or VIBEMDM_ENROLL_TOKEN) are required")
	}
	if !strings.HasPrefix(*server, "https://") && !*insecure {
		return fmt.Errorf("the server URL must use https:// (pass --allow-insecure-http only for a lab server)")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("enroll must be run as root (use sudo)")
	}
	*server = strings.TrimRight(*server, "/")
	si := agent.CollectSystemInfo(ctx)
	client := agent.NewClient(*server, "", version)
	fmt.Printf("Enrolling %s (%s) with %s…\n", si.ComputerName, si.SerialNumber, *server)
	// A reinstall proves it is the same Mac by presenting the existing agent's token.
	prev := ""
	if old, err := agent.LoadConfig(""); err == nil {
		prev = old.DeviceToken
	}
	resp, err := client.Enroll(ctx, model.AgentEnrollRequest{
		Token: *token, SerialNumber: si.SerialNumber, UDID: si.UDID, Hostname: si.Hostname, ComputerName: si.ComputerName,
		OSVersion: si.OSVersion, Architecture: si.Arch, AgentVersion: version, PreviousDeviceToken: prev,
	})
	if err != nil {
		return fmt.Errorf("enrollment failed: %w", err)
	}
	cfg := &agent.Config{ServerURL: *server, DeviceToken: resp.DeviceToken, ComputerID: resp.ComputerID, OrgName: resp.OrgName,
		CheckinIntervalSeconds: resp.CheckinIntervalSeconds, EnrolledAt: time.Now().UTC().Format(time.RFC3339)}
	if err := cfg.Save(""); err != nil {
		return err
	}
	os.MkdirAll(agent.DownloadsDir, 0o700)
	os.MkdirAll(agent.TmpDir, 0o700)
	// Ensure the binary lives at the standard path for launchd.
	bin := currentBinary()
	if bin != agent.BinaryPath {
		if data, err := os.ReadFile(bin); err == nil {
			os.MkdirAll(filepath.Dir(agent.BinaryPath), 0o755)
			if err := os.WriteFile(agent.BinaryPath+".tmp", data, 0o755); err == nil {
				os.Rename(agent.BinaryPath+".tmp", agent.BinaryPath)
				bin = agent.BinaryPath
			}
		}
	}
	if err := agent.InstallLaunchd(bin); err != nil {
		return fmt.Errorf("installing launchd services: %w", err)
	}
	fmt.Printf("Enrolled. Computer ID %s; agent daemon installed.\n", resp.ComputerID)
	if *mdm && !resp.MDMEnrolled {
		prof, err := agent.NewClient(*server, resp.DeviceToken, version).EnrollmentProfile(ctx)
		if err != nil {
			fmt.Printf("Could not download the MDM enrollment profile: %v\n", err)
			return nil
		}
		// Root never writes into a user-controlled directory (symlink attacks):
		// create a fresh file in the sticky /Users/Shared and let the user open it.
		f, err := os.CreateTemp("/Users/Shared", "VibeMDM-Enrollment-*.mobileconfig")
		if err != nil {
			fmt.Printf("Could not save the MDM enrollment profile: %v\n", err)
			return nil
		}
		path := f.Name()
		_, werr := f.Write(prof)
		f.Chmod(0o644)
		f.Close()
		if werr != nil {
			fmt.Printf("Could not save the MDM enrollment profile: %v\n", werr)
			return nil
		}
		if user := agent.ConsoleUser(ctx); user != "" {
			uid := agent.ConsoleUserUID(ctx, user)
			exec.Command("launchctl", "asuser", fmt.Sprint(uid), "sudo", "-u", user, "open", path).Run()
			exec.Command("launchctl", "asuser", fmt.Sprint(uid), "sudo", "-u", user, "open", "x-apple.systempreferences:com.apple.Profiles-Settings.extension").Run()
		}
		fmt.Printf("MDM enrollment profile saved to %q and opened.\nApprove it in System Settings → General → Device Management to complete MDM enrollment.\n", path)
	} else if resp.MDMEnrolled {
		fmt.Println("This Mac is already enrolled in MDM.")
	}
	return nil
}

func run(ctx context.Context, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, name, args...).Output()
	return string(out)
}

func cmdLogs(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	n := fs.Int("n", 50, "number of lines to show")
	follow := fs.Bool("f", false, "follow new lines")
	level := fs.String("level", "debug", "minimum level to show: debug, info, warn, error")
	component := fs.String("component", "", "only show this component")
	grep := fs.String("grep", "", "only show lines containing this text")
	unified := fs.Bool("unified", false, "read macOS unified logging instead of the log file")
	last := fs.String("last", "1h", "with --unified: how far back to read")
	setLevel := fs.String("set-level", "", "change the log level: debug, info, warn, error")
	fs.Parse(args)
	if *n < 1 {
		*n = 1
	}

	if *setLevel != "" {
		l, err := agent.ParseLogLevel(*setLevel)
		if err != nil {
			return err
		}
		return requireRoot(func() error {
			if err := agent.SaveLogLevel("", l); err != nil {
				return err
			}
			if err := agent.SendLocalLogLevel(agent.LevelName(l)); err != nil {
				fmt.Printf("Log level saved as %s; it applies when the daemon next starts (%v).\n", agent.LevelName(l), err)
				return nil
			}
			fmt.Printf("Log level set to %s.\n", agent.LevelName(l))
			return nil
		})
	}

	if *unified {
		if !agent.UnifiedLoggingAvailable() {
			fmt.Fprintln(os.Stderr, "note: this agent was built without unified logging; only older or other builds' records will appear")
		}
		pred := `subsystem == "` + agent.LogSubsystem + `"`
		if *component != "" {
			pred += fmt.Sprintf(` AND category == %q`, *component)
		}
		var c *exec.Cmd
		if *follow {
			c = exec.CommandContext(ctx, "/usr/bin/log", "stream", "--style", "compact", "--level", "debug", "--predicate", pred)
		} else {
			c = exec.CommandContext(ctx, "/usr/bin/log", "show", "--style", "compact", "--info", "--debug", "--last", *last, "--predicate", pred)
		}
		fmt.Fprintln(os.Stderr, "$", strings.Join(c.Args, " "))
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		return c.Run()
	}

	min, err := agent.ParseLogLevel(*level)
	if err != nil {
		return err
	}
	f := agent.LogFilter{MinLevel: min, Component: *component, Contains: *grep}
	lines, err := agent.TailLogs(agent.LogPath, *n, f)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("%w (members of the admin group can read the log; otherwise use sudo)", err)
		}
		return err
	}
	if len(lines) == 0 && !*follow {
		fmt.Fprintf(os.Stderr, "no matching log lines in %s\n", agent.LogPath)
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	if *follow {
		return agent.FollowLogs(ctx, agent.LogPath, os.Stdout, f)
	}
	return nil
}
