package broker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/segfault-stack/media-cookie-broker/internal/providers"
)

var profileIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

var reportKinds = map[string]bool{
	"ok":                      true,
	"authentication_required": true,
	"access_denied":           true,
	"rate_limited":            true,
	"unknown_failure":         true,
}

func ValidProfileID(profile string) bool {
	return profileIDPattern.MatchString(profile)
}

func ValidRole(role string) bool {
	return role == "publisher" || role == "reader" || role == "diagnostics_reader"
}

func ValidReportKind(kind string) bool { return reportKinds[kind] }

func ValidateUpload(provider string, upload *Upload) (string, error) {
	spec, ok := providers.Lookup(provider)
	if !ok {
		return "", errors.New("unknown provider")
	}
	if upload.SchemaVersion != SchemaVersion {
		return "", fmt.Errorf("schema_version must be %d", SchemaVersion)
	}
	if upload.PublicationReason == "" {
		upload.PublicationReason = PublicationOrdinary
	}
	if upload.PublicationReason != PublicationOrdinary && upload.PublicationReason != PublicationRecovery {
		return "", errors.New("publication_reason must be ordinary or recovery")
	}
	if upload.CapturedAt.IsZero() || upload.CapturedAt.After(time.Now().Add(5*time.Minute)) {
		return "", errors.New("captured_at is invalid")
	}
	if len(upload.Cookies) == 0 {
		return "", errors.New("cookies must not be empty")
	}
	if len(upload.Cookies) > 5000 {
		return "", errors.New("too many cookies")
	}
	seen := map[string]bool{}
	for i := range upload.Cookies {
		cookie := &upload.Cookies[i]
		if invalidField(cookie.Domain) || invalidField(cookie.Path) || invalidField(cookie.Name) || invalidField(cookie.Value) {
			return "", fmt.Errorf("cookie %d contains unsafe Netscape field characters", i)
		}
		cookie.Domain = strings.ToLower(strings.TrimSpace(cookie.Domain))
		cookie.Path = strings.TrimSpace(cookie.Path)
		if cookie.Path == "" || !strings.HasPrefix(cookie.Path, "/") {
			return "", fmt.Errorf("cookie %d has invalid path", i)
		}
		if !allowedDomain(cookie.Domain, spec.AllowedDomains) {
			return "", fmt.Errorf("cookie %d has forbidden domain", i)
		}
		if cookie.Name == "" {
			return "", fmt.Errorf("cookie %d has invalid name or value", i)
		}
		if cookie.Expiration < 0 {
			return "", fmt.Errorf("cookie %d has invalid expiration", i)
		}
		switch cookie.SameSite {
		case "", "unspecified", "no_restriction", "lax", "strict":
		default:
			return "", fmt.Errorf("cookie %d has invalid same_site", i)
		}
		key := cookie.Domain + "\x00" + cookie.Path + "\x00" + cookie.Name
		if seen[key] {
			return "", fmt.Errorf("duplicate cookie %q", cookie.Name)
		}
		seen[key] = true
	}
	sort.Slice(upload.Cookies, func(i, j int) bool {
		a, b := upload.Cookies[i], upload.Cookies[j]
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Name < b.Name
	})
	canonical, _ := json.Marshal(upload.Cookies)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func allowedDomain(domain string, allowed []string) bool {
	bare := strings.TrimPrefix(domain, ".")
	for _, root := range allowed {
		if bare == root || strings.HasSuffix(bare, "."+root) {
			return true
		}
	}
	return false
}

func AuthExpiry(provider string, cookies []Cookie) (*time.Time, string) {
	spec, ok := providers.Lookup(provider)
	if !ok || len(spec.AuthCookieNames) == 0 {
		return nil, ""
	}
	authNames := make(map[string]bool, len(spec.AuthCookieNames))
	for _, name := range spec.AuthCookieNames {
		authNames[name] = true
	}
	var earliest time.Time
	for _, cookie := range cookies {
		if !authNames[cookie.Name] || cookie.Expiration <= 0 {
			continue
		}
		expires := time.Unix(cookie.Expiration, 0).UTC()
		if earliest.IsZero() || expires.Before(earliest) {
			earliest = expires
		}
	}
	if earliest.IsZero() {
		return nil, ""
	}
	return &earliest, "provider_auth_cookie"
}

func invalidField(value string) bool { return strings.ContainsAny(value, "\r\n\t") }
