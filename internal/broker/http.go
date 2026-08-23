package broker

import (
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/segfault-stack/media-cookie-broker/internal/providers"
)

const maxUploadBytes = 1 << 20
const maxDiagnosticsCompressedBytes = 1 << 20
const maxDiagnosticsDecompressedBytes = 4 << 20

type API struct {
	store *Store
	auth  *Auth
	log   *slog.Logger
}

func NewHandler(store *Store, auth *Auth, logger *slog.Logger) http.Handler {
	api := &API{store: store, auth: auth, log: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("PUT /v1/providers/{provider}/cookies", api.put)
	mux.HandleFunc("PUT /v1/providers/{provider}/profiles/{profile}/cookies", api.put)
	mux.HandleFunc("GET /v1/providers/{provider}/cookies.txt", api.getCookies)
	mux.HandleFunc("GET /v1/providers/{provider}/profiles/{profile}/cookies.txt", api.getCookies)
	mux.HandleFunc("GET /v1/providers/{provider}/status", api.status)
	mux.HandleFunc("GET /v1/providers/{provider}/profiles/{profile}/status", api.status)
	mux.HandleFunc("POST /v1/providers/{provider}/reports", api.putReport)
	mux.HandleFunc("POST /v1/providers/{provider}/profiles/{profile}/reports", api.putReport)
	mux.HandleFunc("GET /v1/status", api.allStatus)
	mux.HandleFunc("POST /v1/diagnostics/events", api.putDiagnostics)
	mux.HandleFunc("GET /v1/diagnostics/events", api.getDiagnostics)
	return securityHeaders(mux)
}

func (a *API) putDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Encoding") != "gzip" {
		problem(w, http.StatusUnsupportedMediaType, "gzip content encoding is required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDiagnosticsCompressedBytes)
	reader, err := gzip.NewReader(r.Body)
	if err != nil {
		a.log.Warn("diagnostic batch rejected", "reason", "invalid_gzip")
		problem(w, http.StatusBadRequest, "invalid gzip payload")
		return
	}
	defer reader.Close()
	dec := json.NewDecoder(io.LimitReader(reader, maxDiagnosticsDecompressedBytes+1))
	dec.DisallowUnknownFields()
	var batch DiagnosticBatch
	if err := dec.Decode(&batch); err != nil {
		a.log.Warn("diagnostic batch rejected", "reason", "invalid_json")
		problem(w, http.StatusBadRequest, "invalid diagnostics payload")
		return
	}
	if err := ensureEOF(dec); err != nil {
		problem(w, http.StatusBadRequest, "payload must contain one JSON object")
		return
	}
	if err := ValidateDiagnostics(batch.Provider, &batch); err != nil {
		a.log.Warn("diagnostic batch rejected", "provider", batch.Provider, "reason", err.Error())
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if _, ok := a.authorized(r, "publisher", batch.Provider, batch.Profile); !ok {
		unauthorized(w)
		return
	}
	count, err := a.store.PutDiagnostics(r.Context(), batch.Provider, batch.Profile, batch)
	if err != nil {
		a.log.Error("diagnostic batch storage failed", "provider", batch.Provider, "installation_id", batch.InstallationID, "error", err)
		problem(w, http.StatusInternalServerError, "storage failure")
		return
	}
	a.log.Info("diagnostic batch accepted", "provider", batch.Provider, "profile", batch.Profile, "installation_id", batch.InstallationID, "events", count)
	jsonResponse(w, http.StatusCreated, map[string]any{"accepted": count})
}

func (a *API) getDiagnostics(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if !providers.ValidID(provider) {
		problem(w, http.StatusBadRequest, "valid provider is required")
		return
	}
	profile := r.URL.Query().Get("profile")
	if profile == "" {
		profile = DefaultProfile
	}
	if !ValidProfileID(profile) {
		problem(w, http.StatusBadRequest, "valid profile is required")
		return
	}
	if _, ok := a.authorized(r, "diagnostics_reader", provider, profile); !ok {
		unauthorized(w)
		return
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	from, err := diagnosticTime(r.URL.Query().Get("from"))
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid from timestamp")
		return
	}
	to, err := diagnosticTime(r.URL.Query().Get("to"))
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid to timestamp")
		return
	}
	records, err := a.store.Diagnostics(r.Context(), provider, profile, r.URL.Query().Get("installation_id"), r.URL.Query().Get("severity"), r.URL.Query().Get("event_type"), from, to, before, limit)
	if err != nil {
		a.log.Error("diagnostics read", "provider", provider, "error", err)
		problem(w, http.StatusInternalServerError, "storage failure")
		return
	}
	next := int64(0)
	if len(records) > 0 {
		next = records[len(records)-1].ID
	}
	jsonResponse(w, http.StatusOK, map[string]any{"events": records, "next_before_id": next})
}

func diagnosticTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Ping(r.Context()); err != nil {
		problem(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) put(w http.ResponseWriter, r *http.Request) {
	provider, profile, ok := requestScope(w, r)
	if !ok {
		return
	}
	if _, ok := a.authorized(r, "publisher", provider, profile); !ok {
		unauthorized(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var upload Upload
	if err := dec.Decode(&upload); err != nil {
		problem(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if err := ensureEOF(dec); err != nil {
		problem(w, http.StatusBadRequest, "payload must contain one JSON object")
		return
	}
	hash, err := ValidateUpload(provider, &upload)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	authExpiresAt, authExpirySource := AuthExpiry(provider, upload.Cookies)
	status, err := a.store.Put(r.Context(), provider, profile, hash, upload.PublicationReason, upload.CapturedAt, upload.Cookies, authExpiresAt, authExpirySource)
	if err != nil {
		a.log.Error("store upload", "provider", provider, "error", err)
		problem(w, http.StatusInternalServerError, "storage failure")
		return
	}
	code := http.StatusOK
	if status.Changed {
		code = http.StatusCreated
	}
	w.Header().Set("Location", statusPath(provider, profile))
	jsonResponse(w, code, status)
}

func (a *API) getCookies(w http.ResponseWriter, r *http.Request) {
	provider, profile, ok := requestScope(w, r)
	if !ok {
		return
	}
	username, ok := a.authorized(r, "reader", provider, profile)
	if !ok {
		unauthorized(w)
		return
	}
	snapshot, err := a.store.Current(r.Context(), provider, profile)
	if err != nil {
		currentError(w, err)
		return
	}
	etag := `"` + snapshot.Status.SHA256 + `"`
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Cookie-Revision", strconv.FormatInt(snapshot.Status.Revision, 10))
	w.Header().Set("X-Cookie-Captured-At", snapshot.Status.CapturedAt.Format(time.RFC3339))
	w.Header().Set("X-Cookie-Created-At", snapshot.Status.CreatedAt.Format(time.RFC3339))
	w.Header().Set("X-Cookie-Provider", provider)
	w.Header().Set("X-Cookie-Profile", profile)
	if err := a.store.RecordConsumerSeen(r.Context(), username, provider, profile, snapshot.Status.Revision); err != nil {
		a.log.Error("record consumer activity", "provider", provider, "profile", profile, "consumer", username, "error", err)
		problem(w, http.StatusInternalServerError, "storage failure")
		return
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, renderNetscape(snapshot.Cookies))
}

func (a *API) status(w http.ResponseWriter, r *http.Request) {
	provider, profile, ok := requestScope(w, r)
	if !ok {
		return
	}
	username, password, ok := r.BasicAuth()
	if !ok || (!a.auth.Authenticate(r.Context(), username, password, "reader", provider, profile) && !a.auth.Authenticate(r.Context(), username, password, "publisher", provider, profile)) {
		unauthorized(w)
		return
	}
	snapshot, err := a.store.Current(r.Context(), provider, profile)
	if err != nil {
		currentError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonResponse(w, http.StatusOK, snapshot.Status)
}

func (a *API) allStatus(w http.ResponseWriter, r *http.Request) {
	username, password, ok := r.BasicAuth()
	if !ok || !a.auth.AuthenticateRole(r.Context(), username, password, "publisher") {
		unauthorized(w)
		return
	}
	scopes, err := a.store.ScopesForUser(r.Context(), username)
	if err != nil {
		problem(w, http.StatusInternalServerError, "storage failure")
		return
	}
	statuses, err := a.store.StatusesForScopes(r.Context(), scopes)
	if err != nil {
		a.log.Error("list publisher status", "publisher", username, "error", err)
		problem(w, http.StatusInternalServerError, "storage failure")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"profiles": statuses})
}

func (a *API) putReport(w http.ResponseWriter, r *http.Request) {
	provider, profile, ok := requestScope(w, r)
	if !ok {
		return
	}
	username, ok := a.authorized(r, "reader", provider, profile)
	if !ok {
		unauthorized(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var report HealthReport
	if err := dec.Decode(&report); err != nil || ensureEOF(dec) != nil {
		problem(w, http.StatusBadRequest, "invalid health report")
		return
	}
	if report.Revision <= 0 || !ValidReportKind(report.Kind) {
		problem(w, http.StatusUnprocessableEntity, "invalid revision or report kind")
		return
	}
	before, _ := a.store.Current(r.Context(), provider, profile)
	if err := a.store.PutHealthReport(r.Context(), username, provider, profile, report); err != nil {
		a.log.Warn("consumer health report rejected", "provider", provider, "profile", profile, "revision", report.Revision, "consumer", username, "kind", report.Kind, "reason", err.Error())
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	a.log.Info("consumer health report accepted", "provider", provider, "profile", profile, "revision", report.Revision, "consumer", username, "kind", report.Kind)
	after, err := a.store.Current(r.Context(), provider, profile)
	if err == nil && before.Status.AuthHealth != after.Status.AuthHealth {
		a.log.Info("provider profile health transition", "provider", provider, "profile", profile, "revision", after.Status.Revision, "previous_health_state", before.Status.AuthHealth, "health_state", after.Status.AuthHealth)
	}
	jsonResponse(w, http.StatusOK, map[string]any{"accepted": true, "consumer": username, "provider": provider, "profile": profile, "revision": report.Revision, "kind": report.Kind})
}

func (a *API) authorized(r *http.Request, role, provider, profile string) (string, bool) {
	username, password, ok := r.BasicAuth()
	return username, ok && a.auth.Authenticate(r.Context(), username, password, role, provider, profile)
}

func requestScope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	provider := r.PathValue("provider")
	if !providers.ValidID(provider) {
		problem(w, http.StatusNotFound, "unknown provider")
		return "", "", false
	}
	profile := r.PathValue("profile")
	if profile == "" {
		profile = DefaultProfile
	}
	if !ValidProfileID(profile) {
		problem(w, http.StatusBadRequest, "invalid profile")
		return "", "", false
	}
	return provider, profile, true
}

func statusPath(provider, profile string) string {
	if profile == DefaultProfile {
		return fmt.Sprintf("/v1/providers/%s/status", provider)
	}
	return fmt.Sprintf("/v1/providers/%s/profiles/%s/status", provider, profile)
}

func currentError(w http.ResponseWriter, err error) {
	if err == sql.ErrNoRows {
		problem(w, http.StatusNotFound, "no cookie snapshot published")
		return
	}
	problem(w, http.StatusInternalServerError, "storage failure")
}
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="cookie-broker", charset="UTF-8"`)
	problem(w, http.StatusUnauthorized, "authentication required")
}
func problem(w http.ResponseWriter, code int, detail string) {
	jsonResponse(w, code, map[string]any{"status": code, "detail": detail})
}
func jsonResponse(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
func ensureEOF(dec *json.Decoder) error {
	var extra any
	err := dec.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	return fmt.Errorf("extra JSON")
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func renderNetscape(cookies []Cookie) string {
	var b strings.Builder
	b.WriteString("# Netscape HTTP Cookie File\n# Generated by media-cookie-broker; do not edit.\n")
	for _, c := range cookies {
		domain := c.Domain
		if c.HTTPOnly {
			domain = "#HttpOnly_" + domain
		}
		includeSubdomains := "FALSE"
		if strings.HasPrefix(c.Domain, ".") {
			includeSubdomains = "TRUE"
		}
		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", domain, includeSubdomains, c.Path, secure, c.Expiration, c.Name, c.Value)
	}
	return b.String()
}
