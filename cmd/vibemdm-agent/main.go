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

Example:
  sudo VIBEMDM_ENROLL_TOKEN=… vibemdm-agent enroll --server https://mdm.example.com --mdm`,
	"status": `vibemdm-agent status

Prints the server URL, computer ID, agent version, whether the daemon is
running, whether the server is reachable with this Mac's credentials, and the
state of local components (swiftDialog, super, remote shell opt-out). Safe to
run without sudo, but some checks need root.`,
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

Opens the local Self Service portal (http://127.0.0.1:8571) for the logged-in user.`,
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
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd, args := os.Args[1], os.Args[2:]
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
	case "enroll":
		err = cmdEnroll(ctx, args, logger)
	case "daemon":
		err = requireRoot(func() error {
			cfg, err := agent.LoadConfig("")
			if err != nil {
				return err
			}
			// Logs go to the local file (via launchd stdout) and are shipped to the server.
			ship := agent.NewLogShipper(agent.NewClient(cfg.ServerURL, cfg.DeviceToken, version), slog.LevelInfo)
			dlog := slog.New(ship.Wrap(logger.Handler()))
			d := agent.NewDaemon(cfg, version, dlog)
			d.Ship = ship
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
		user := agent.ConsoleUser(ctx)
		url := "http://" + agent.SelfServicePort
		if user != "" && os.Geteuid() == 0 {
			uid := agent.ConsoleUserUID(ctx, user)
			err = exec.Command("launchctl", "asuser", fmt.Sprint(uid), "sudo", "-u", user, "open", url).Run()
		} else {
			err = exec.Command("open", url).Run()
		}
		if err == nil {
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
	fs.Parse(args)
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
