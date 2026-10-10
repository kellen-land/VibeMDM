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

const usage = `VibeMDM agent %s

Usage: vibemdm-agent <command> [options]

Commands:
  enroll            Enroll this Mac with a VibeMDM server and install the launchd services
                      --server URL   VibeMDM server URL (https://mdm.example.com)
                      --token TOKEN  Enrollment token (invitation or enrollment secret)
                      --mdm          Also download and open the MDM enrollment profile
  daemon            Run the agent daemon (used by launchd)
  policy            Check in and run policies now
                      --event NAME   Trigger to fire (checkin, startup, login, enrollment_complete, or a custom event)
  trigger <event>   Fire a trigger via the running daemon (used by the login LaunchAgent)
  recon             Collect inventory and submit it to the server
  checkin           Perform one check-in cycle
  check-connection  Verify the server is reachable
  selfservice       Open the Self Service portal in the default browser
  manage            (Re)install the launchd daemon and login agent
  remove-framework  Unenroll and remove the agent from this Mac
  version           Print the agent version
`

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
		fmt.Printf(usage, version)
	case "enroll":
		err = cmdEnroll(ctx, args, logger)
	case "daemon":
		err = requireRoot(func() error {
			cfg, err := agent.LoadConfig("")
			if err != nil {
				return err
			}
			return agent.NewDaemon(cfg, version, logger).Run(ctx)
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
	token := fs.String("token", "", "enrollment token")
	mdm := fs.Bool("mdm", false, "also open the MDM enrollment profile")
	fs.Parse(args)
	if *server == "" || *token == "" {
		return fmt.Errorf("--server and --token are required")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("enroll must be run as root (use sudo)")
	}
	*server = strings.TrimRight(*server, "/")
	si := agent.CollectSystemInfo(ctx)
	client := agent.NewClient(*server, "", version)
	fmt.Printf("Enrolling %s (%s) with %s…\n", si.ComputerName, si.SerialNumber, *server)
	resp, err := client.Enroll(ctx, model.AgentEnrollRequest{
		Token: *token, SerialNumber: si.SerialNumber, UDID: si.UDID, Hostname: si.Hostname, ComputerName: si.ComputerName,
		OSVersion: si.OSVersion, Architecture: si.Arch, AgentVersion: version,
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
		user := agent.ConsoleUser(ctx)
		dir := "/Users/Shared"
		if user != "" {
			if h := strings.TrimSpace(run(ctx, "dscl", ".", "-read", "/Users/"+user, "NFSHomeDirectory")); strings.Contains(h, "/") {
				dir = filepath.Join(strings.TrimSpace(strings.TrimPrefix(h, "NFSHomeDirectory:")), "Downloads")
			}
		}
		path := filepath.Join(dir, cfg.OrgName+" Enrollment.mobileconfig")
		if err := os.WriteFile(path, prof, 0o644); err != nil {
			path = "/Users/Shared/VibeMDM Enrollment.mobileconfig"
			os.WriteFile(path, prof, 0o644)
		}
		if user != "" {
			exec.Command("chown", user, path).Run()
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
