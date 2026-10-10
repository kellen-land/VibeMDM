// Package agent implements the VibeMDM macOS management agent.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

// Client talks to the VibeMDM agent API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	Version string
}

// NewClient creates a client.
func NewClient(baseURL, token, version string) *Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConnsPerHost = 4
	tr.IdleConnTimeout = 120 * time.Second
	tr.ForceAttemptHTTP2 = true
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, Version: version, HTTP: &http.Client{Timeout: 90 * time.Second, Transport: tr}}
}

type apiError struct {
	Status int
	Msg    string
}

func (e *apiError) Error() string { return fmt.Sprintf("server returned %d: %s", e.Status, e.Msg) }

func asAPIError(err error, target **apiError) bool { return errors.As(err, target) }

// IsUnauthorized reports whether the error is a 401 (token revoked).
func IsUnauthorized(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.Status == http.StatusUnauthorized
}

func (c *Client) do(ctx context.Context, method, path string, in, out any, timeout time.Duration) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("User-Agent", "vibemdm-agent/"+c.Version)
	client := c.HTTP
	if timeout > 0 {
		cl := *c.HTTP
		cl.Timeout = timeout
		client = &cl
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(data))
		}
		return &apiError{Status: resp.StatusCode, Msg: e.Error}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Enroll registers the device.
func (c *Client) Enroll(ctx context.Context, req model.AgentEnrollRequest) (*model.AgentEnrollResponse, error) {
	var out model.AgentEnrollResponse
	if err := c.do(ctx, http.MethodPost, "/agent/v1/enroll", req, &out, 0); err != nil {
		return nil, err
	}
	return &out, nil
}

// Checkin performs a check-in.
func (c *Client) Checkin(ctx context.Context, req model.AgentCheckinRequest) (*model.AgentCheckinResponse, error) {
	var out model.AgentCheckinResponse
	if err := c.do(ctx, http.MethodPost, "/agent/v1/checkin", req, &out, 0); err != nil {
		return nil, err
	}
	return &out, nil
}

// PollTasks long-polls for tasks and on-demand check-in requests.
func (c *Client) PollTasks(ctx context.Context, waitSecs int) (*model.AgentPollResponse, error) {
	var out model.AgentPollResponse
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/agent/v1/tasks?wait=%d", waitSecs), nil, &out, time.Duration(waitSecs+30)*time.Second); err != nil {
		return nil, err
	}
	return &out, nil
}

// SubmitTaskResult reports a task result.
func (c *Client) SubmitTaskResult(ctx context.Context, id string, res model.AgentTaskResult) error {
	return c.do(ctx, http.MethodPost, "/agent/v1/tasks/"+id+"/result", res, nil, 0)
}

// SubmitPolicyLog reports a policy run.
func (c *Client) SubmitPolicyLog(ctx context.Context, logID string, sub model.AgentPolicyLogSubmit) error {
	return c.do(ctx, http.MethodPost, "/agent/v1/policy-logs/"+logID, sub, nil, 0)
}

// ReconConfig fetches extension attribute scripts.
func (c *Client) ReconConfig(ctx context.Context) (*model.AgentReconConfig, error) {
	var out model.AgentReconConfig
	if err := c.do(ctx, http.MethodGet, "/agent/v1/recon-config", nil, &out, 0); err != nil {
		return nil, err
	}
	return &out, nil
}

// SubmitInventory uploads inventory.
func (c *Client) SubmitInventory(ctx context.Context, inv model.AgentInventory) error {
	return c.do(ctx, http.MethodPost, "/agent/v1/inventory", inv, nil, 5*time.Minute)
}

// Unenroll removes the agent enrollment server-side.
func (c *Client) Unenroll(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/agent/v1/unenroll", map[string]any{}, nil, 0)
}

// ServerVersion returns the server version (no auth).
func (c *Client) ServerVersion(ctx context.Context) (string, error) {
	var out struct {
		Version string `json:"version"`
	}
	if err := c.do(ctx, http.MethodGet, "/agent/v1/version", nil, &out, 15*time.Second); err != nil {
		return "", err
	}
	return out.Version, nil
}

// EnrollmentProfile downloads the MDM enrollment profile.
func (c *Client) EnrollmentProfile(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/agent/v1/enrollment-profile", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != 200 {
		return nil, &apiError{Status: resp.StatusCode, Msg: strings.TrimSpace(string(data))}
	}
	return data, nil
}

// SubmitComplianceScan posts the results of an mSCP compliance scan.
func (c *Client) SubmitComplianceScan(ctx context.Context, scanID string, sub model.AgentComplianceSubmit) error {
	return c.do(ctx, "POST", "/agent/v1/compliance-scans/"+scanID, sub, nil, 2*time.Minute)
}

// SubmitAppInstallResult reports an app catalog install outcome.
func (c *Client) SubmitAppInstallResult(ctx context.Context, logID, status, logText string) error {
	return c.do(ctx, http.MethodPost, "/agent/v1/app-installs/"+logID+"/result", map[string]string{"status": status, "log": logText}, nil, 0)
}

// ReportRestricted reports a restricted-software violation.
func (c *Client) ReportRestricted(ctx context.Context, rep model.AgentRestrictedReport) error {
	return c.do(ctx, http.MethodPost, "/agent/v1/restricted-events", rep, nil, 0)
}

// SelfServiceList fetches the Self Service items for this device.
func (c *Client) SelfServiceList(ctx context.Context) (items []model.SelfServiceItem, orgName string, err error) {
	var out struct {
		Items   []model.SelfServiceItem `json:"items"`
		OrgName string                  `json:"org_name"`
	}
	err = c.do(ctx, http.MethodGet, "/agent/v1/selfservice", nil, &out, 0)
	return out.Items, out.OrgName, err
}

// SelfServiceRun asks the server to prepare a self-service item.
func (c *Client) SelfServiceRun(ctx context.Context, kind, id string) (*model.AgentPolicy, *model.AgentAppInstall, error) {
	var out struct {
		Policy *model.AgentPolicy     `json:"policy"`
		App    *model.AgentAppInstall `json:"app"`
	}
	if err := c.do(ctx, http.MethodPost, "/agent/v1/selfservice/run", map[string]string{"kind": kind, "id": id}, &out, 0); err != nil {
		return nil, nil, err
	}
	return out.Policy, out.App, nil
}

// DownloadTo fetches an authenticated path to a local file.
func (c *Client) DownloadTo(ctx context.Context, urlPath, dst string) error {
	return c.DownloadPackage(ctx, urlPath, dst)
}

// DownloadPackage streams a package to dst.
func (c *Client) DownloadPackage(ctx context.Context, urlPath, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+urlPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	cl := *c.HTTP
	cl.Timeout = 2 * time.Hour
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &apiError{Status: resp.StatusCode, Msg: strings.TrimSpace(string(data))}
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// toWS converts an http(s) base URL to a ws(s) URL.
func toWS(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasPrefix(base, "https://") {
		return "wss://" + strings.TrimPrefix(base, "https://")
	}
	if strings.HasPrefix(base, "http://") {
		return "ws://" + strings.TrimPrefix(base, "http://")
	}
	return base
}

// AuthHeaderFor is authHeader, exported for the device simulator.
func AuthHeaderFor(token, version string) http.Header { return authHeader(token, version) }

// authHeader builds the device authorization header set for WebSocket dials.
func authHeader(token, version string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	h.Set("User-Agent", "vibemdm-agent/"+version)
	return h
}

// ShipLogs uploads a batch of agent log records.
func (c *Client) ShipLogs(ctx context.Context, entries []model.AgentLogEntry) error {
	return c.do(ctx, http.MethodPost, "/agent/v1/logs", map[string]any{"entries": entries}, nil, 30*time.Second)
}

// UploadDiagnostics PUTs a gzipped tarball for a requested bundle.
func (c *Client) UploadDiagnostics(ctx context.Context, bundleID, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, _ := f.Stat()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.BaseURL+"/agent/v1/diagnostics/"+bundleID, f)
	if err != nil {
		return err
	}
	req.ContentLength = fi.Size()
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	cl := *c.HTTP
	cl.Timeout = 30 * time.Minute
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &apiError{Status: resp.StatusCode, Msg: strings.TrimSpace(string(b))}
	}
	return nil
}

// DiagnosticsFailed reports that a bundle could not be collected.
func (c *Client) DiagnosticsFailed(ctx context.Context, bundleID, msg string) error {
	return c.do(ctx, http.MethodPost, "/agent/v1/diagnostics/"+bundleID+"/failed", map[string]string{"error": msg}, nil, 0)
}
