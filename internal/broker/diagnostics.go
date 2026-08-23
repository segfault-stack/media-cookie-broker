package broker

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/segfault-stack/media-cookie-broker/internal/providers"
)

const maxDiagnosticEvents = 1000
const maxDiagnosticDepth = 4

var diagnosticCredentialText = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*\S+|cookie\s*:\s*[^;\s=]+=[^;\s]+|BEGIN [A-Z ]*PRIVATE KEY|\b(bearer|basic)\s+[A-Za-z0-9+/._~=-]{8,}|\b(token|access_token|api_key|apikey|secret|password|passwd|master_key|private_key)\s*=\s*[^\s&,;]+)`)

func ValidateDiagnostics(provider string, batch *DiagnosticBatch) error {
	if batch.Provider != provider {
		return errors.New("provider mismatch")
	}
	if !providers.ValidID(provider) {
		return errors.New("unknown provider")
	}
	if batch.Profile == "" {
		batch.Profile = DefaultProfile
	}
	if !ValidProfileID(batch.Profile) {
		return errors.New("invalid profile")
	}
	if batch.SchemaVersion != DiagnosticsSchemaVersion {
		return fmt.Errorf("schema_version must be %d", DiagnosticsSchemaVersion)
	}
	if len(batch.InstallationID) < 8 || len(batch.InstallationID) > 128 || invalidField(batch.InstallationID) {
		return errors.New("invalid installation_id")
	}
	if len(batch.Events) == 0 || len(batch.Events) > maxDiagnosticEvents {
		return errors.New("invalid event count")
	}
	now := time.Now().UTC()
	for i, event := range batch.Events {
		if event.Timestamp.IsZero() || event.Timestamp.After(now.Add(5*time.Minute)) || event.Timestamp.Before(now.Add(-31*24*time.Hour)) {
			return fmt.Errorf("event %d has invalid timestamp", i)
		}
		if len(event.Type) == 0 || len(event.Type) > 100 || invalidField(event.Type) {
			return fmt.Errorf("event %d has invalid type", i)
		}
		if event.Severity != "info" && event.Severity != "warning" && event.Severity != "error" {
			return fmt.Errorf("event %d has invalid severity", i)
		}
		if event.Details == nil {
			event.Details = map[string]any{}
		}
		if eventProvider, present := event.Details["provider"]; present && eventProvider != provider {
			return fmt.Errorf("event %d has mismatched provider", i)
		}
		if eventProfile, present := event.Details["profile"]; present && eventProfile != batch.Profile {
			return fmt.Errorf("event %d has mismatched profile", i)
		}
		if err := validateDetails(event.Details); err != nil {
			return fmt.Errorf("event %d: %w", i, err)
		}
	}
	return nil
}

func validateDetails(value map[string]any) error {
	return validateDiagnosticObject(value, 0)
}

func validateDiagnosticObject(value map[string]any, depth int) error {
	if depth > maxDiagnosticDepth {
		return errors.New("diagnostic value too deeply nested")
	}
	for key, item := range value {
		if len(key) > 100 || invalidField(key) || forbiddenDiagnosticKey(key) {
			return errors.New("forbidden diagnostic field")
		}
		if err := validateDiagnosticValue(item, depth+1); err != nil {
			return err
		}
	}
	return nil
}
func validateDiagnosticValue(value any, depth int) error {
	if depth > maxDiagnosticDepth {
		return errors.New("diagnostic value too deeply nested")
	}
	switch item := value.(type) {
	case string:
		if len(item) > 4096 || diagnosticCredentialText.MatchString(item) {
			return errors.New("invalid diagnostic text")
		}
	case float64, bool, nil:
		return nil
	case []any:
		if len(item) > 500 {
			return errors.New("diagnostic array too large")
		}
		for _, child := range item {
			if err := validateDiagnosticValue(child, depth+1); err != nil {
				return err
			}
		}
	case map[string]any:
		if err := validateDiagnosticObject(item, depth); err != nil {
			return err
		}
	default:
		return errors.New("invalid diagnostic value")
	}
	return nil
}

func forbiddenDiagnosticKey(key string) bool {
	normalized := normalizeDiagnosticKey(key)
	compact := strings.ReplaceAll(normalized, "_", "")
	parts := strings.FieldsFunc(normalized, func(r rune) bool { return r == '_' })
	for _, part := range parts {
		if part == "password" || part == "passwd" || part == "authorization" || part == "secret" {
			return true
		}
	}
	for i := 0; i+1 < len(parts); i++ {
		pair := parts[i] + "_" + parts[i+1]
		switch pair {
		case "access_token", "refresh_token", "bearer_token", "id_token", "api_key", "auth_header",
			"basic_auth", "bearer_auth", "basic_credentials", "bearer_credentials", "cookie_value",
			"cookie_header", "master_key", "private_key":
			return true
		}
	}
	if normalized == "token" || strings.HasSuffix(normalized, "_token") ||
		normalized == "secret" || strings.HasSuffix(normalized, "_secret") ||
		normalized == "api_key" || normalized == "apikey" || strings.HasSuffix(normalized, "_api_key") || strings.HasSuffix(normalized, "_apikey") ||
		normalized == "auth_header" || strings.HasSuffix(normalized, "_auth_header") ||
		normalized == "basic_auth" || strings.HasSuffix(normalized, "_basic_auth") ||
		normalized == "bearer_auth" || strings.HasSuffix(normalized, "_bearer_auth") ||
		normalized == "basic_credentials" || strings.HasSuffix(normalized, "_basic_credentials") ||
		normalized == "bearer_credentials" || strings.HasSuffix(normalized, "_bearer_credentials") ||
		normalized == "cookie" || normalized == "cookies" || strings.HasSuffix(normalized, "_cookie") || strings.HasSuffix(normalized, "_cookies") ||
		normalized == "cookie_value" || strings.HasSuffix(normalized, "_cookie_value") ||
		normalized == "cookie_header" || strings.HasSuffix(normalized, "_cookie_header") ||
		normalized == "master_key" || strings.HasSuffix(normalized, "_master_key") ||
		normalized == "private_key" || strings.HasSuffix(normalized, "_private_key") ||
		strings.Contains(compact, "apikey") || strings.Contains(compact, "cookievalue") ||
		strings.Contains(compact, "cookieheader") || strings.Contains(compact, "masterkey") ||
		strings.Contains(compact, "privatekey") || strings.Contains(compact, "accesstoken") ||
		strings.Contains(compact, "refreshtoken") || strings.Contains(compact, "bearertoken") {
		return true
	}
	return false
}

func normalizeDiagnosticKey(key string) string {
	var out strings.Builder
	for i, r := range key {
		if unicode.IsUpper(r) && i > 0 {
			out.WriteByte('_')
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(unicode.ToLower(r))
		} else if out.Len() > 0 {
			out.WriteByte('_')
		}
	}
	return strings.Trim(out.String(), "_")
}
