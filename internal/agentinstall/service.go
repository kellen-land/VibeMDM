package agentinstall

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	nanoapi "github.com/micromdm/nanomdm/api"
	"github.com/micromdm/nanomdm/mdm"

	"github.com/vibemdm/vibemdm/internal/auth"
	"github.com/vibemdm/vibemdm/internal/commands"
	"github.com/vibemdm/vibemdm/internal/model"
	"github.com/vibemdm/vibemdm/internal/store"
)

// Enqueuer is the part of the MDM commander the service needs.
type Enqueuer interface {
	Enqueue(ctx context.Context, comp *model.Computer, cmd *mdm.Command, kind, refID, createdBy string) (*nanoapi.APIResult, error)
}

const (
	pkgFileName  = "VibeMDM-Agent.pkg"
	maxPkgSize   = 200 << 20
	md5ChunkSize = 10 << 20
	tokenTTL     = 24 * time.Hour
	// ManifestPath and PackagePath are the public URLs mdmclient downloads from.
	ManifestPath = "/agent/pkg/manifest.plist"
	PackagePath  = "/agent/pkg/" + pkgFileName
)

// Service installs the agent through MDM.
type Service struct {
	st      *store.Store
	cmdr    Enqueuer
	dataDir string
	log     *slog.Logger
	build   buildRun

	// Wake (optional) tells an online agent to pick up new tasks now.
	Wake func(computerID string)
}

// New creates the service. Packages are kept in dataDir/agent-package.
func New(st *store.Store, cmdr Enqueuer, dataDir string, logger *slog.Logger) *Service {
	return &Service{st: st, cmdr: cmdr, dataDir: dataDir, log: logger}
}

func (s *Service) pkgPath() string { return filepath.Join(s.dataDir, "agent-package", pkgFileName) }

// CA returns the package-signing CA, creating it on first use.
func (s *Service) CA(ctx context.Context) (certPEM, keyPEM string, err error) {
	certPEM, keyPEM, err = s.st.GetPkgSigningCA(ctx)
	if err == nil {
		if CAIsCurrent(certPEM) {
			return certPEM, keyPEM, nil
		}
		// Made by an earlier version: its certificates carry the code-signing usage, which
		// productsign refuses, so no working package can exist under it. Replace it.
		settings, _ := s.st.GetSettings(ctx)
		c, k, err := NewCA(settings.OrgName)
		if err != nil {
			return "", "", err
		}
		if err := s.st.ReplacePkgSigningCA(ctx, c, k); err != nil {
			return "", "", err
		}
		s.st.DeleteAgentPackage(ctx)
		os.Remove(s.pkgPath())
		s.log.Warn("replaced the package-signing CA: certificates from the old one could not sign installer packages; build the agent package again",
			"fingerprint", Fingerprint(c))
		return c, k, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", "", err // never replace a CA we failed to read
	}
	settings, _ := s.st.GetSettings(ctx)
	c, k, err := NewCA(settings.OrgName)
	if err != nil {
		return "", "", err
	}
	if err := s.st.CreatePkgSigningCA(ctx, c, k); err != nil && !errors.Is(err, store.ErrCAExists) {
		return "", "", err
	}
	s.log.Info("created package-signing CA")
	return s.st.GetPkgSigningCA(ctx)
}

// Package returns the uploaded package record if its file is present.
func (s *Service) Package(ctx context.Context) (*model.AgentPackage, error) {
	p, err := s.st.GetAgentPackage(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(s.pkgPath()); err != nil {
		return nil, store.ErrNotFound
	}
	return p, nil
}

var versionRe = regexp.MustCompile(`^[0-9A-Za-z._+-]{1,40}$`)

// Upload stores a signed agent package. Unsigned packages are refused: macOS only installs
// signed packages through InstallEnterpriseApplication.
func (s *Service) Upload(ctx context.Context, r io.Reader, version, by string) (*model.AgentPackage, error) {
	if !versionRe.MatchString(version) {
		return nil, errors.New("version must be 1-40 letters, digits, dots, dashes or underscores")
	}
	dir := filepath.Dir(s.pkgPath())
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(dir, "upload-*.pkg")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	sh := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, sh), io.LimitReader(r, maxPkgSize+1))
	if err != nil {
		return nil, err
	}
	if n > maxPkgSize {
		return nil, fmt.Errorf("package is larger than %d MB", maxPkgSize>>20)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	signer, err := PackageSigner(tmp)
	if err != nil {
		return nil, err
	}
	md5s, err := chunkMD5s(tmp, md5ChunkSize)
	if err != nil {
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp.Name(), s.pkgPath()); err != nil {
		return nil, err
	}
	p := model.AgentPackage{FileName: pkgFileName, Version: version, Size: n, SHA256: hex.EncodeToString(sh.Sum(nil)),
		MD5Chunk: md5ChunkSize, MD5s: md5s, SignedBy: signer, UploadedBy: by}
	if err := s.st.SaveAgentPackage(ctx, p); err != nil {
		return nil, err
	}
	return s.st.GetAgentPackage(ctx)
}

func chunkMD5s(f io.ReadSeeker, chunk int64) ([]string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var out []string
	for {
		h := md5.New()
		n, err := io.CopyN(h, f, chunk)
		if n > 0 {
			out = append(out, hex.EncodeToString(h.Sum(nil)))
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

var certRe = regexp.MustCompile(`(?s)<X509Certificate>(.*?)</X509Certificate>`)

// PackageSigner checks that r is a flat package (xar archive) with a signature and returns
// the signing certificate's common name.
func PackageSigner(r io.ReadSeeker) (string, error) {
	var hdr struct {
		Magic       [4]byte
		Size        uint16
		Version     uint16
		TOCLen      uint64
		TOCLenPlain uint64
		CksumAlg    uint32
	}
	if err := binary.Read(r, binary.BigEndian, &hdr); err != nil || string(hdr.Magic[:]) != "xar!" {
		return "", errors.New("not a flat installer package (.pkg)")
	}
	if hdr.TOCLen == 0 || hdr.TOCLen > 16<<20 || hdr.TOCLenPlain > 64<<20 {
		return "", errors.New("package table of contents is invalid")
	}
	if _, err := r.Seek(int64(hdr.Size), io.SeekStart); err != nil {
		return "", err
	}
	zr, err := zlib.NewReader(io.LimitReader(r, int64(hdr.TOCLen)))
	if err != nil {
		return "", errors.New("package table of contents is unreadable")
	}
	toc, err := io.ReadAll(io.LimitReader(zr, int64(hdr.TOCLenPlain)+1))
	if err != nil {
		return "", errors.New("package table of contents is unreadable")
	}
	if !bytes.Contains(toc, []byte("<signature")) {
		return "", errors.New("the package is not signed; macOS installs only signed packages through MDM (see deploy/macos/build-agent-pkg.sh)")
	}
	m := certRe.FindSubmatch(toc)
	if m == nil {
		return "", errors.New("the package signature has no certificate")
	}
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(m[1])), ""))
	if err != nil {
		return "", errors.New("the package signing certificate is unreadable")
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		return "", errors.New("the package signing certificate is unreadable")
	}
	return c.Subject.CommonName, nil
}

// ErrNoPackage means no signed agent package has been uploaded yet.
var ErrNoPackage = errors.New("no signed agent package has been uploaded (Settings → Enrollment → Automatic agent install)")

// Install sends the bootstrap profile and InstallEnterpriseApplication to a Mac.
func (s *Service) Install(ctx context.Context, comp *model.Computer, by string) error {
	if !comp.MDMEnrolled {
		return errors.New("the Mac is not enrolled in MDM")
	}
	pkg, err := s.Package(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNoPackage
		}
		return err
	}
	settings, err := s.st.GetSettings(ctx)
	if err != nil {
		return err
	}
	if settings.ServerURL == "" {
		return errors.New("the server URL is not configured")
	}
	caPEM, _, err := s.CA(ctx)
	if err != nil {
		return err
	}
	token := "vmbt_" + auth.RandomToken(24)
	if err := s.st.CreateBootstrapToken(ctx, comp.ID, store.HashToken(token), tokenTTL); err != nil {
		return err
	}
	prof, err := BootstrapProfile(settings.OrgName, caPEM, settings.ServerURL, token)
	if err != nil {
		return err
	}
	pc, err := commands.InstallProfile(prof)
	if err != nil {
		return err
	}
	if _, err := s.cmdr.Enqueue(ctx, comp, pc, "agent_bootstrap", "", by); err != nil {
		return err
	}
	ic, err := commands.InstallEnterpriseApplication(strings.TrimRight(settings.ServerURL, "/") + ManifestPath)
	if err != nil {
		return err
	}
	if _, err := s.cmdr.Enqueue(ctx, comp, ic, "agent_install", "", by); err != nil {
		return err
	}
	if err := s.st.RecordAgentInstall(ctx, comp.ID, ic.CommandUUID, by); err != nil {
		return err
	}
	s.st.Audit(ctx, by, "agent.mdm_install", "computer", comp.ID, map[string]any{"version": pkg.Version, "command_uuid": ic.CommandUUID})
	s.log.Info("agent install sent through MDM", "computer", comp.Name, "version", pkg.Version)
	return nil
}

// AutoInstall runs after a new MDM enrollment when the setting is on and the agent is missing.
func (s *Service) AutoInstall(ctx context.Context, comp *model.Computer) {
	settings, err := s.st.GetSettings(ctx)
	if err != nil || !settings.AutoInstallAgent || comp.AgentEnrolled {
		return
	}
	if err := s.Install(ctx, comp, "system"); err != nil {
		s.log.Warn("automatic agent install not sent", "computer", comp.Name, "err", err)
	}
}

// AgentEnrolled removes the bootstrap profile once the agent is set up.
func (s *Service) AgentEnrolled(ctx context.Context, comp *model.Computer) {
	has, err := s.st.HasBootstrap(ctx, comp.ID)
	if err != nil || !has {
		return
	}
	s.st.MarkAgentInstalled(ctx, comp.ID)
	if comp.MDMEnrolled {
		cmd, err := commands.RemoveProfile(BootstrapIdentifier)
		if err == nil {
			if _, err := s.cmdr.Enqueue(ctx, comp, cmd, "agent_bootstrap_remove", "", "system"); err != nil {
				s.log.Warn("removing the agent bootstrap profile", "computer", comp.Name, "err", err)
				return
			}
		}
	}
	s.st.ClearBootstrap(ctx, comp.ID)
}

// CommandResult records the Mac's answer to an InstallEnterpriseApplication command.
func (s *Service) CommandResult(ctx context.Context, commandUUID, status, errText string) {
	st, detail := "sent", "the Mac accepted the install; waiting for the agent to enroll"
	if status != "Acknowledged" {
		st, detail = "failed", errText
	}
	s.st.UpdateAgentInstall(ctx, commandUUID, st, detail)
}

// Register mounts the public package routes used by mdmclient.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+ManifestPath, s.manifest)
	mux.HandleFunc("GET "+PackagePath, s.download)
}

func (s *Service) manifest(w http.ResponseWriter, r *http.Request) {
	pkg, err := s.Package(r.Context())
	if err != nil {
		http.Error(w, "no agent package", http.StatusNotFound)
		return
	}
	settings, _ := s.st.GetSettings(r.Context())
	title := settings.OrgName
	if title == "" {
		title = "VibeMDM"
	}
	b, err := Manifest(strings.TrimRight(settings.ServerURL, "/")+PackagePath, pkg.Size, pkg.MD5Chunk, pkg.MD5s, pkg.Version, title+" Agent")
	if err != nil {
		http.Error(w, "manifest error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Write(b)
}

// download serves the package. It holds no secrets: the enrollment token travels only
// in the bootstrap profile, over the Mac's MDM channel.
func (s *Service) download(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open(s.pkgPath())
	if err != nil {
		http.Error(w, "no agent package", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, pkgFileName, st.ModTime(), f)
}
