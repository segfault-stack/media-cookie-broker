package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type target struct {
	provider, profile, path, etag string
	content                       []byte
	sidecar                       []byte
	metadata                      bool
}

var providerIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
var profileIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type cookieMetadata struct {
	SchemaVersion int    `json:"schema_version"`
	Provider      string `json:"provider"`
	Profile       string `json:"profile"`
	Revision      int64  `json:"revision"`
	SHA256        string `json:"sha256"`
	CapturedAt    string `json:"captured_at,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) > 1 && os.Args[1] == "report" {
		fatal(logger, runReportCLI(context.Background(), &http.Client{Timeout: 20 * time.Second}, os.Args[2:]))
		return
	}
	targets, err := parseTargets(os.Getenv("COOKIE_SYNC_TARGETS"))
	fatal(logger, err)
	metadata, err := parseBool(env("COOKIE_SYNC_METADATA", "true"))
	fatal(logger, err)
	for _, target := range targets {
		target.metadata = metadata
	}
	passwordPath, err := requiredEnv("BROKER_PASSWORD_FILE")
	fatal(logger, err)
	password, err := os.ReadFile(passwordPath)
	fatal(logger, err)
	password = trim(password)
	if len(password) == 0 {
		fatal(logger, errors.New("broker password file is empty"))
	}
	interval, err := time.ParseDuration(env("COOKIE_SYNC_INTERVAL", "5m"))
	fatal(logger, err)
	if interval < 10*time.Second {
		fatal(logger, errors.New("COOKIE_SYNC_INTERVAL must be at least 10s"))
	}
	brokerURL, err := requiredEnv("BROKER_URL")
	fatal(logger, err)
	base, err := parseBaseURL(brokerURL)
	fatal(logger, err)
	username, err := requiredEnv("BROKER_USERNAME")
	fatal(logger, err)
	combined := strings.TrimSpace(os.Getenv("COOKIE_SYNC_COMBINED"))
	fatal(logger, validateCombined(combined, targets))
	client := &http.Client{Timeout: 20 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	run := func() {
		if err := syncAll(ctx, client, base, username, string(password), targets, combined); err != nil {
			logger.Error("cookie sync failed", "error", err)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func syncAll(ctx context.Context, client *http.Client, base, username, password string, targets []*target, combined string) error {
	var failures []error
	for _, t := range targets {
		if err := fetch(ctx, client, base, username, password, t); err != nil {
			failures = append(failures, fmt.Errorf("%s/%s: %w", t.provider, t.profile, err))
		}
	}
	if combined != "" {
		var data []byte
		included := 0
		for _, t := range targets {
			content := lastGoodContent(t)
			if len(content) == 0 {
				continue
			}
			if included == 0 {
				data = append(data, content...)
			} else if headerEnd := bytes.IndexByte(content, '\n'); headerEnd >= 0 {
				data = append(data, content[headerEnd+1:]...)
			}
			included++
		}
		if included > 0 {
			if err := atomicWrite(combined, data); err != nil {
				failures = append(failures, fmt.Errorf("combined: %w", err))
			}
		}
	}
	return errors.Join(failures...)
}

func lastGoodContent(t *target) []byte {
	if validCookieFile(t.content) {
		return t.content
	}
	data, err := os.ReadFile(t.path)
	if err == nil && validCookieFile(data) {
		return data
	}
	return nil
}

func validCookieFile(data []byte) bool {
	return bytes.HasPrefix(data, []byte("# Netscape HTTP Cookie File\n"))
}

func fetch(ctx context.Context, client *http.Client, base, username, password string, t *target) error {
	if t.profile == "" {
		t.profile = "default"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scopeURL(base, t.provider, t.profile, "cookies.txt"), nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(username, password)
	if t.etag != "" {
		req.Header.Set("If-None-Match", t.etag)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		if t.etag == "" {
			return errors.New("broker returned 304 to an unconditional request")
		}
		if localMatchesTrustedContent(t) {
			return nil
		}
		t.etag = ""
		return fetch(ctx, client, base, username, password, t)
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("broker returned %s", resp.Status)
	}
	const maxCookieFileBytes = 2 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCookieFileBytes+1))
	if err != nil {
		return err
	}
	if !validCookieFile(data) {
		return errors.New("invalid Netscape cookie file")
	}
	if len(data) > maxCookieFileBytes {
		return errors.New("cookie file exceeds 2 MiB")
	}
	if err := atomicWrite(t.path, data); err != nil {
		return err
	}
	var sidecar []byte
	if t.metadata {
		sidecar, err = writeMetadataFromResponse(t, data, resp.Header)
		if err != nil {
			return fmt.Errorf("write metadata sidecar: %w", err)
		}
	}
	t.content = data
	t.sidecar = sidecar
	t.etag = resp.Header.Get("ETag")
	return nil
}

func localMatchesTrustedContent(t *target) bool {
	if !validCookieFile(t.content) {
		return false
	}
	data, err := os.ReadFile(t.path)
	if err != nil || !bytes.Equal(data, t.content) {
		return false
	}
	if !t.metadata {
		return true
	}
	raw, err := os.ReadFile(metadataPath(t.path))
	return err == nil && len(t.sidecar) > 0 && bytes.Equal(raw, t.sidecar)
}

func parseTargets(raw string) ([]*target, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("COOKIE_SYNC_TARGETS is required")
	}
	var out []*target
	scopes := map[string]bool{}
	paths := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid target %q", item)
		}
		scope, path := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		provider, profile, err := parseScope(scope)
		if err != nil || !filepath.IsAbs(path) || filepath.Clean(path) == string(filepath.Separator) {
			return nil, fmt.Errorf("invalid target %q", item)
		}
		path = filepath.Clean(path)
		scopeKey := provider + "/" + profile
		if scopes[scopeKey] || paths[path] || paths[metadataPath(path)] {
			return nil, fmt.Errorf("duplicate target %q", item)
		}
		for existingPath := range paths {
			if metadataPath(existingPath) == path {
				return nil, fmt.Errorf("target %q collides with a metadata sidecar", item)
			}
		}
		scopes[scopeKey] = true
		paths[path] = true
		out = append(out, &target{provider: provider, profile: profile, path: path})
	}
	return out, nil
}

func validProviderID(provider string) bool { return providerIDPattern.MatchString(provider) }

func validProfileID(profile string) bool { return profileIDPattern.MatchString(profile) }

func parseScope(value string) (string, string, error) {
	parts := strings.Split(value, "/")
	if len(parts) > 2 || !validProviderID(parts[0]) {
		return "", "", errors.New("invalid provider/profile")
	}
	profile := "default"
	if len(parts) == 2 {
		profile = parts[1]
	}
	if !validProfileID(profile) {
		return "", "", errors.New("invalid profile")
	}
	return parts[0], profile, nil
}

func scopeURL(base, provider, profile, resource string) string {
	providerPath := url.PathEscape(provider)
	if profile == "default" {
		return base + "/v1/providers/" + providerPath + "/" + resource
	}
	return base + "/v1/providers/" + providerPath + "/profiles/" + url.PathEscape(profile) + "/" + resource
}

func metadataPath(cookiePath string) string { return cookiePath + ".meta.json" }

func writeMetadataFromResponse(t *target, data []byte, header http.Header) ([]byte, error) {
	revision, err := strconv.ParseInt(header.Get("X-Cookie-Revision"), 10, 64)
	if err != nil || revision <= 0 {
		return nil, errors.New("broker response is missing a valid X-Cookie-Revision")
	}
	if provider := header.Get("X-Cookie-Provider"); provider != "" && provider != t.provider {
		return nil, errors.New("broker metadata provider mismatch")
	}
	if profile := header.Get("X-Cookie-Profile"); profile != "" && profile != t.profile {
		return nil, errors.New("broker metadata profile mismatch")
	}
	sum := sha256.Sum256(data)
	metadata := cookieMetadata{
		SchemaVersion: 1,
		Provider:      t.provider,
		Profile:       t.profile,
		Revision:      revision,
		SHA256:        hex.EncodeToString(sum[:]),
		CapturedAt:    header.Get("X-Cookie-Captured-At"),
		CreatedAt:     header.Get("X-Cookie-Created-At"),
	}
	raw, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	if err := atomicWrite(metadataPath(t.path), raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func runReportCLI(ctx context.Context, client *http.Client, args []string) error {
	fs := flag.NewFlagSet("cookie-sync report", flag.ContinueOnError)
	provider := fs.String("provider", "", "provider")
	profile := fs.String("profile", "default", "profile")
	cookieFile := fs.String("file", "", "Netscape cookie file")
	metadataFile := fs.String("metadata-file", "", "metadata sidecar; defaults to <file>.meta.json")
	kind := fs.String("kind", "", "normalized report kind")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !validProviderID(*provider) || !validProfileID(*profile) || !filepath.IsAbs(*cookieFile) || *kind == "" {
		return errors.New("valid --provider, --profile, absolute --file, and --kind are required")
	}
	path := *metadataFile
	if path == "" {
		path = metadataPath(filepath.Clean(*cookieFile))
	}
	brokerURL := strings.TrimSpace(os.Getenv("BROKER_URL"))
	username := strings.TrimSpace(os.Getenv("BROKER_USERNAME"))
	passwordPath := strings.TrimSpace(os.Getenv("BROKER_PASSWORD_FILE"))
	if brokerURL == "" || username == "" || passwordPath == "" {
		return errors.New("BROKER_URL, BROKER_USERNAME, and BROKER_PASSWORD_FILE are required")
	}
	base, err := parseBaseURL(brokerURL)
	if err != nil {
		return err
	}
	password, err := os.ReadFile(passwordPath)
	if err != nil {
		return err
	}
	password = trim(password)
	if len(password) == 0 {
		return errors.New("broker password file is empty")
	}
	revision, err := reportHealth(ctx, client, base, username, string(password), *provider, *profile, filepath.Clean(*cookieFile), path, *kind)
	if err != nil {
		return err
	}
	fmt.Printf("health report accepted for %s/%s revision %d (%s)\n", *provider, *profile, revision, *kind)
	return nil
}

func reportHealth(ctx context.Context, client *http.Client, base, username, password, provider, profile, cookiePath, sidecarPath, kind string) (int64, error) {
	data, err := os.ReadFile(cookiePath)
	if err != nil {
		return 0, fmt.Errorf("read cookie file: %w", err)
	}
	if !validCookieFile(data) {
		return 0, errors.New("cookie file is not a valid Netscape file")
	}
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return 0, fmt.Errorf("read metadata sidecar: %w", err)
	}
	if len(raw) > 16<<10 {
		return 0, errors.New("metadata sidecar is too large")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var metadata cookieMetadata
	if err := dec.Decode(&metadata); err != nil {
		return 0, fmt.Errorf("decode metadata sidecar: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return 0, errors.New("metadata sidecar must contain one JSON object")
	}
	if metadata.SchemaVersion != 1 || metadata.Provider != provider || metadata.Profile != profile || metadata.Revision <= 0 {
		return 0, errors.New("metadata sidecar does not match requested provider/profile")
	}
	sum := sha256.Sum256(data)
	if hashMismatch(metadata.SHA256, hex.EncodeToString(sum[:])) {
		return 0, errors.New("metadata sidecar SHA256 does not match cookie file; refusing to report")
	}
	body, _ := json.Marshal(map[string]any{"revision": metadata.Revision, "kind": kind})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scopeURL(base, provider, profile, "reports"), bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send health report: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("broker rejected health report: %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	return metadata.Revision, nil
}

func hashMismatch(expected, actual string) bool {
	return len(expected) != len(actual) || !strings.EqualFold(expected, actual)
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", value)
	}
}

func parseBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("BROKER_URL must be an absolute URL without credentials, query, or fragment")
	}
	host := strings.ToLower(parsed.Hostname())
	loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return "", errors.New("BROKER_URL must use HTTPS (HTTP is allowed only for loopback)")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validateCombined(path string, targets []*target) error {
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) == string(filepath.Separator) {
		return errors.New("COOKIE_SYNC_COMBINED must be an absolute file path")
	}
	clean := filepath.Clean(path)
	for _, target := range targets {
		if target.path == clean || metadataPath(target.path) == clean {
			return errors.New("COOKIE_SYNC_COMBINED must differ from provider targets and metadata sidecars")
		}
	}
	return nil
}
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".cookie-sync-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}
func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func trim(value []byte) []byte {
	for len(value) > 0 && (value[len(value)-1] == '\n' || value[len(value)-1] == '\r') {
		value = value[:len(value)-1]
	}
	return value
}
func fatal(log *slog.Logger, err error) {
	if err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}
