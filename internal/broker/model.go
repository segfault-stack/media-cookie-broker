package broker

import "time"

const SchemaVersion = 1
const DefaultProfile = "default"

type PublicationReason string

const (
	PublicationOrdinary PublicationReason = "ordinary"
	PublicationRecovery PublicationReason = "recovery"
)

type Cookie struct {
	Domain     string `json:"domain"`
	Path       string `json:"path"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	Expiration int64  `json:"expiration"`
	Secure     bool   `json:"secure"`
	HTTPOnly   bool   `json:"http_only"`
	SameSite   string `json:"same_site"`
}

type Upload struct {
	SchemaVersion     int               `json:"schema_version"`
	PublicationReason PublicationReason `json:"publication_reason,omitempty"`
	CapturedAt        time.Time         `json:"captured_at"`
	Cookies           []Cookie          `json:"cookies"`
}

type Status struct {
	Provider            string            `json:"provider"`
	Profile             string            `json:"profile"`
	Revision            int64             `json:"revision"`
	SHA256              string            `json:"sha256"`
	CookieCount         int               `json:"cookie_count"`
	CapturedAt          time.Time         `json:"captured_at"`
	CreatedAt           time.Time         `json:"created_at"`
	PublicationReason   PublicationReason `json:"publication_reason"`
	Changed             bool              `json:"changed,omitempty"`
	AuthHealth          string            `json:"auth_health"`
	AuthExpiresAt       *time.Time        `json:"auth_expires_at,omitempty"`
	AuthExpirySource    string            `json:"auth_expiry_source,omitempty"`
	AuthRequiredCount   int               `json:"auth_required_count"`
	LastReportAt        *time.Time        `json:"last_report_at,omitempty"`
	CurrentReportCounts map[string]int    `json:"current_report_counts,omitempty"`
}

type storedSnapshot struct {
	Status  Status
	Cookies []Cookie
}

const DiagnosticsSchemaVersion = 1

// DiagnosticBatch deliberately contains only redacted operational fields.
type DiagnosticBatch struct {
	SchemaVersion  int               `json:"schema_version"`
	Provider       string            `json:"provider"`
	Profile        string            `json:"profile,omitempty"`
	InstallationID string            `json:"installation_id"`
	Events         []DiagnosticEvent `json:"events"`
}
type DiagnosticEvent struct {
	Timestamp time.Time      `json:"timestamp"`
	Type      string         `json:"type"`
	Severity  string         `json:"severity"`
	Details   map[string]any `json:"details"`
}
type DiagnosticRecord struct {
	ID             int64           `json:"id"`
	Provider       string          `json:"provider"`
	Profile        string          `json:"profile"`
	InstallationID string          `json:"installation_id"`
	Event          DiagnosticEvent `json:"event"`
}

type Scope struct {
	Provider string `json:"provider"`
	Profile  string `json:"profile"`
}

type UserRecord struct {
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Scopes    []Scope   `json:"scopes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type HealthReport struct {
	Revision int64  `json:"revision"`
	Kind     string `json:"kind"`
}

type ConsumerState struct {
	Username     string    `json:"username"`
	Provider     string    `json:"provider"`
	Profile      string    `json:"profile"`
	LastSeen     time.Time `json:"last_seen"`
	RevisionSeen int64     `json:"revision_seen"`
}
