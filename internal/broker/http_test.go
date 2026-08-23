package broker

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testAPI(t *testing.T) (http.Handler, *Store) {
	t.Helper()
	key := bytes.Repeat([]byte{7}, 32)
	store, err := OpenStore(filepath.Join(t.TempDir(), "db.sqlite3"), key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	for _, user := range []struct{ username, role string }{{"writer", "publisher"}, {"reader", "reader"}, {"diagnostics", "diagnostics_reader"}} {
		if err := store.CreateUser(context.Background(), user.username, "correct horse battery staple", user.role, []Scope{{Provider: "youtube", Profile: DefaultProfile}}); err != nil {
			t.Fatal(err)
		}
	}
	return NewHandler(store, NewAuth(store), slog.New(slog.NewTextHandler(ioDiscard{}, nil))), store
}

func gzipBody(t *testing.T, value any) *bytes.Reader {
	t.Helper()
	var body bytes.Buffer
	writer := gzip.NewWriter(&body)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(body.Bytes())
}

func TestDiagnosticsRoleSeparationAndEncryptedStorage(t *testing.T) {
	h, store := testAPI(t)
	batch := DiagnosticBatch{SchemaVersion: 1, Provider: "youtube", InstallationID: "extension-installation-123", Events: []DiagnosticEvent{{Timestamp: time.Now().UTC(), Type: "sync_succeeded", Severity: "info", Details: map[string]any{"provider": "youtube", "cookie_count": float64(2)}}}}
	request := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/events", gzipBody(t, batch))
	request.Header.Set("Content-Encoding", "gzip")
	request.SetBasicAuth("writer", "correct horse battery staple")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("diagnostics upload=%d %s", response.Code, response.Body.String())
	}
	var ciphertext []byte
	if err := store.db.QueryRow(`SELECT ciphertext FROM diagnostics LIMIT 1`).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("sync_succeeded")) {
		t.Fatal("diagnostics were stored in plaintext")
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/diagnostics/events?provider=youtube", nil)
	request.SetBasicAuth("reader", "correct horse battery staple")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("cookie reader diagnostics=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/diagnostics/events?provider=youtube&event_type=sync_succeeded", nil)
	request.SetBasicAuth("diagnostics", "correct horse battery staple")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "sync_succeeded") {
		t.Fatalf("diagnostics read=%d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/providers/youtube/cookies.txt", nil)
	request.SetBasicAuth("diagnostics", "correct horse battery staple")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("diagnostics reader cookie read=%d", response.Code)
	}
}

func TestDiagnosticsRejectsMalformedAndForbiddenFields(t *testing.T) {
	h, _ := testAPI(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/events", strings.NewReader("not gzip"))
	request.Header.Set("Content-Encoding", "gzip")
	request.SetBasicAuth("writer", "correct horse battery staple")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("bad gzip=%d", response.Code)
	}
	batch := DiagnosticBatch{SchemaVersion: 1, Provider: "youtube", InstallationID: "extension-installation-123", Events: []DiagnosticEvent{{Timestamp: time.Now(), Type: "bad", Severity: "error", Details: map[string]any{"password": "must-not-pass"}}}}
	request = httptest.NewRequest(http.MethodPost, "/v1/diagnostics/events", gzipBody(t, batch))
	request.Header.Set("Content-Encoding", "gzip")
	request.SetBasicAuth("writer", "correct horse battery staple")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("forbidden field=%d %s", response.Code, response.Body.String())
	}
	batch = DiagnosticBatch{SchemaVersion: 1, Provider: "youtube", Profile: "default", InstallationID: "extension-installation-123", Events: []DiagnosticEvent{{Timestamp: time.Now(), Type: "bad_scope", Severity: "error", Details: map[string]any{"provider": "youtube", "profile": "private-account"}}}}
	request = httptest.NewRequest(http.MethodPost, "/v1/diagnostics/events", gzipBody(t, batch))
	request.Header.Set("Content-Encoding", "gzip")
	request.SetBasicAuth("writer", "correct horse battery staple")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatched diagnostic profile=%d %s", response.Code, response.Body.String())
	}
}

func TestDiagnosticsRejectsCredentialKeysAndTextButAllowsOperationalDetails(t *testing.T) {
	base := func(details map[string]any) DiagnosticBatch {
		return DiagnosticBatch{
			SchemaVersion:  1,
			Provider:       "youtube",
			Profile:        DefaultProfile,
			InstallationID: "extension-installation-123",
			Events: []DiagnosticEvent{{
				Timestamp: time.Now().UTC(),
				Type:      "validation_test",
				Severity:  "warning",
				Details:   details,
			}},
		}
	}
	for _, key := range []string{
		"password", "passwd", "requestAuthorization", "auth_header", "basic_credentials", "bearer_auth", "cookie", "cookieValue", "cookie_header",
		"token", "accessToken", "access_token_expiry", "client_secret", "client_secret_status", "api_key", "apiKeyPresent", "apikey", "brokerMasterKey", "private-key",
	} {
		batch := base(map[string]any{key: "not-a-real-secret"})
		if err := ValidateDiagnostics("youtube", &batch); err == nil {
			t.Errorf("credential-bearing diagnostic key %q was accepted", key)
		}
	}
	for _, text := range []string{
		"Authorization: Basic abcdefghijklmnop",
		"Cookie: SID=not-a-real-cookie",
		"request failed with Bearer abcdefghijklmnop",
		"request?token=not-a-real-token",
		"access_token=not-a-real-token",
		"api_key=not-a-real-key",
		"secret=not-a-real-secret",
		"password=not-a-real-password",
		"master_key=not-a-real-master-key",
		"-----BEGIN PRIVATE KEY----- not-a-real-key",
	} {
		batch := base(map[string]any{"error": text})
		if err := ValidateDiagnostics("youtube", &batch); err == nil {
			t.Errorf("credential-shaped diagnostic text %q was accepted", text)
		}
	}
	allowed := base(map[string]any{
		"provider":           "youtube",
		"profile":            DefaultProfile,
		"cookie_names":       []any{"SID"},
		"cookie_domains":     []any{".youtube.com"},
		"auth_health":        "refresh_required",
		"token_bucket_state": "exhausted",
		"http_status":        float64(401),
		"error":              "authentication health check failed",
	})
	if err := ValidateDiagnostics("youtube", &allowed); err != nil {
		t.Fatalf("safe operational diagnostic details were rejected: %v", err)
	}
}

func TestDiagnosticsDepthLimitAcrossObjectsArraysAndMixedValues(t *testing.T) {
	validate := func(details map[string]any) error {
		batch := DiagnosticBatch{
			SchemaVersion:  1,
			Provider:       "youtube",
			Profile:        DefaultProfile,
			InstallationID: "extension-installation-123",
			Events: []DiagnosticEvent{{
				Timestamp: time.Now().UTC(),
				Type:      "depth_test",
				Severity:  "info",
				Details:   details,
			}},
		}
		return ValidateDiagnostics("youtube", &batch)
	}

	for name, details := range map[string]map[string]any{
		"objects_at_limit": {"one": map[string]any{"two": map[string]any{"three": map[string]any{"four": "safe"}}}},
		"arrays_at_limit":  {"one": []any{[]any{[]any{"safe"}}}},
		"mixed_at_limit":   {"one": []any{map[string]any{"two": []any{"safe"}}}},
	} {
		if err := validate(details); err != nil {
			t.Errorf("%s was rejected at the depth limit: %v", name, err)
		}
	}
	for name, details := range map[string]map[string]any{
		"objects_beyond_limit": {"one": map[string]any{"two": map[string]any{"three": map[string]any{"four": map[string]any{"five": "safe"}}}}},
		"arrays_beyond_limit":  {"one": []any{[]any{[]any{[]any{[]any{"safe"}}}}}},
		"mixed_beyond_limit":   {"one": []any{map[string]any{"two": []any{map[string]any{"three": []any{"safe"}}}}}},
	} {
		if err := validate(details); err == nil {
			t.Errorf("%s bypassed the depth limit", name)
		}
	}
	if err := validate(map[string]any{"one": map[string]any{"two": map[string]any{"password": "not-a-real-password"}}}); err == nil {
		t.Fatal("sensitive field near the depth limit was accepted")
	}
	if err := validate(map[string]any{"one": map[string]any{"two": map[string]any{"three": map[string]any{"four": map[string]any{"password": "not-a-real-password"}}}}}); err == nil {
		t.Fatal("deep sensitive structure was accepted after reaching the depth limit")
	}
}

func TestDiagnosticsAreProfileScoped(t *testing.T) {
	h, store := testAPI(t)
	batch := DiagnosticBatch{SchemaVersion: 1, Provider: "youtube", Profile: "music-bot", InstallationID: "extension-installation-123", Events: []DiagnosticEvent{{Timestamp: time.Now().UTC(), Type: "profile_event", Severity: "info", Details: map[string]any{"provider": "youtube", "profile": "music-bot"}}}}
	request := httptest.NewRequest(http.MethodPost, "/v1/diagnostics/events", gzipBody(t, batch))
	request.Header.Set("Content-Encoding", "gzip")
	request.SetBasicAuth("writer", "correct horse battery staple")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("default grant uploaded named-profile diagnostics: %d", response.Code)
	}
	if err := store.Grant(context.Background(), "writer", "youtube", "music-bot"); err != nil {
		t.Fatal(err)
	}
	if err := store.Grant(context.Background(), "diagnostics", "youtube", "music-bot"); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/diagnostics/events", gzipBody(t, batch))
	request.Header.Set("Content-Encoding", "gzip")
	request.SetBasicAuth("writer", "correct horse battery staple")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("named-profile diagnostics upload: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/diagnostics/events?provider=youtube&profile=music-bot", nil)
	request.SetBasicAuth("diagnostics", "correct horse battery staple")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "profile_event") {
		t.Fatalf("named-profile diagnostics read: %d %s", response.Code, response.Body.String())
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func TestUploadAuthAndRead(t *testing.T) {
	h, _ := testAPI(t)
	capturedAt := time.Now().UTC().Truncate(time.Second)
	upload := Upload{SchemaVersion: 1, CapturedAt: capturedAt, Cookies: []Cookie{{Domain: ".youtube.com", Path: "/", Name: "SID", Value: "not-a-real-cookie", Expiration: 0, Secure: true, HTTPOnly: true, SameSite: "unspecified"}}}
	body, _ := json.Marshal(upload)
	request := httptest.NewRequest(http.MethodPut, "/v1/providers/youtube/cookies", bytes.NewReader(body))
	request.SetBasicAuth("writer", "correct horse battery staple")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("put=%d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/providers/youtube/cookies.txt", nil)
	request.SetBasicAuth("writer", "correct horse battery staple")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("writer read=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/providers/youtube/cookies.txt", nil)
	request.SetBasicAuth("reader", "correct horse battery staple")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "#HttpOnly_.youtube.com") || !strings.Contains(response.Body.String(), "not-a-real-cookie") {
		t.Fatalf("read=%d %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("X-Cookie-Captured-At"); got != capturedAt.Format(time.RFC3339) {
		t.Fatalf("captured metadata = %q, want %q", got, capturedAt.Format(time.RFC3339))
	}
	if got := response.Header().Get("X-Cookie-Created-At"); got == "" {
		t.Fatal("created metadata is missing")
	}
	etag := response.Header().Get("ETag")
	request = httptest.NewRequest(http.MethodGet, "/v1/providers/youtube/cookies.txt", nil)
	request.SetBasicAuth("reader", "correct horse battery staple")
	request.Header.Set("If-None-Match", etag)
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusNotModified {
		t.Fatalf("conditional=%d", response.Code)
	}
	if got := response.Header().Get("X-Cookie-Captured-At"); got != capturedAt.Format(time.RFC3339) {
		t.Fatalf("304 captured metadata = %q, want %q", got, capturedAt.Format(time.RFC3339))
	}
}

func TestHealthIncludesStorageAvailability(t *testing.T) {
	handler, store := testAPI(t)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("healthy store returned %d", response.Code)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("closed store returned %d", response.Code)
	}
}

func TestValidationRejectsForbiddenDomainAndDuplicate(t *testing.T) {
	upload := Upload{SchemaVersion: 1, CapturedAt: time.Now(), Cookies: []Cookie{{Domain: "evil.example", Path: "/", Name: "SID", Value: "x"}}}
	if _, err := ValidateUpload("youtube", &upload); err == nil {
		t.Fatal("expected forbidden domain")
	}
	upload.Cookies = []Cookie{{Domain: "youtube.com", Path: "/", Name: "SID", Value: "x"}, {Domain: "youtube.com", Path: "/", Name: "SID", Value: "y"}}
	if _, err := ValidateUpload("youtube", &upload); err == nil {
		t.Fatal("expected duplicate")
	}
}

func TestCanonicalCookieHashUsesTupleOrdering(t *testing.T) {
	first := Cookie{Domain: "youtube.com", Path: "/a", Name: "bc", Value: "fake-cookie-a"}
	second := Cookie{Domain: "youtube.com", Path: "/ab", Name: "c", Value: "fake-cookie-b"}
	if first.Domain+first.Path+first.Name != second.Domain+second.Path+second.Name {
		t.Fatal("test cookies do not reproduce the ambiguous concatenated sort key")
	}

	forward := Upload{SchemaVersion: 1, CapturedAt: time.Now(), Cookies: []Cookie{first, second}}
	reverse := Upload{SchemaVersion: 1, CapturedAt: time.Now(), Cookies: []Cookie{second, first}}
	forwardHash, err := ValidateUpload("youtube", &forward)
	if err != nil {
		t.Fatal(err)
	}
	reverseHash, err := ValidateUpload("youtube", &reverse)
	if err != nil {
		t.Fatal(err)
	}
	if forwardHash != reverseHash {
		t.Fatalf("canonical hashes differ by input order: %s != %s", forwardHash, reverseHash)
	}
}

func TestNetscapeSerializationRejectsUnsafeTextFields(t *testing.T) {
	for _, control := range []string{"\t", "\r", "\n"} {
		upload := Upload{SchemaVersion: 1, CapturedAt: time.Now(), Cookies: []Cookie{{Domain: "youtube.com", Path: "/safe" + control + "injected", Name: "SID", Value: "not-a-real-cookie"}}}
		if _, err := ValidateUpload("youtube", &upload); err == nil || strings.Contains(err.Error(), "not-a-real-cookie") {
			t.Fatalf("unsafe path %q was not rejected safely: %v", control, err)
		}
	}
	for name, cookie := range map[string]Cookie{
		"domain": {Domain: "youtube.com\tFALSE", Path: "/", Name: "SID", Value: "fake"},
		"name":   {Domain: "youtube.com", Path: "/", Name: "SID\nextra", Value: "fake"},
		"value":  {Domain: "youtube.com", Path: "/", Name: "SID", Value: "fake\rnext-row"},
	} {
		upload := Upload{SchemaVersion: 1, CapturedAt: time.Now(), Cookies: []Cookie{cookie}}
		if _, err := ValidateUpload("youtube", &upload); err == nil {
			t.Errorf("unsafe %s was accepted", name)
		}
	}
	valid := Upload{SchemaVersion: 1, CapturedAt: time.Now(), Cookies: []Cookie{{Domain: ".youtube.com", Path: "/watch", Name: "SID", Value: "fake"}}}
	if _, err := ValidateUpload("youtube", &valid); err != nil {
		t.Fatal(err)
	}
	serialized := renderNetscape(valid.Cookies)
	if strings.Count(serialized, "\n") != 3 {
		t.Fatalf("one cookie produced an unexpected row count: %q", serialized)
	}
	row := strings.Split(strings.TrimSuffix(serialized, "\n"), "\n")[2]
	if fields := strings.Split(row, "\t"); len(fields) != 7 {
		t.Fatalf("one cookie produced %d Netscape columns: %q", len(fields), row)
	}
}

func TestYouTubeValidationUsesMinimalCookieScope(t *testing.T) {
	for _, domain := range []string{"youtube.com", ".youtube.com", "www.youtube.com"} {
		upload := Upload{SchemaVersion: 1, CapturedAt: time.Now(), Cookies: []Cookie{{Domain: domain, Path: "/", Name: "SID", Value: "fake"}}}
		if _, err := ValidateUpload("youtube", &upload); err != nil {
			t.Fatalf("expected %q to be accepted: %v", domain, err)
		}
	}
	for _, domain := range []string{"google.com", ".accounts.google.com", "youtu.be"} {
		upload := Upload{SchemaVersion: 1, CapturedAt: time.Now(), Cookies: []Cookie{{Domain: domain, Path: "/", Name: "SID", Value: "fake"}}}
		if _, err := ValidateUpload("youtube", &upload); err == nil {
			t.Fatalf("expected %q to be rejected", domain)
		}
	}
}

func TestEncryptedAtRestAndRollback(t *testing.T) {
	_, store := testAPI(t)
	cookies := []Cookie{{Domain: "youtube.com", Path: "/", Name: "SID", Value: "fake-cookie-v1"}}
	hash, _ := ValidateUpload("youtube", &Upload{SchemaVersion: 1, CapturedAt: time.Now(), Cookies: cookies})
	first, err := store.Put(context.Background(), "youtube", DefaultProfile, hash, PublicationOrdinary, time.Now(), cookies, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	cookies[0].Value = "fake-cookie-v2"
	hash, _ = ValidateUpload("youtube", &Upload{SchemaVersion: 1, CapturedAt: time.Now(), Cookies: cookies})
	if _, err = store.Put(context.Background(), "youtube", DefaultProfile, hash, PublicationOrdinary, time.Now(), cookies, nil, ""); err != nil {
		t.Fatal(err)
	}
	var ciphertext []byte
	if err := store.db.QueryRow(`SELECT ciphertext FROM revisions WHERE provider='youtube' AND revision=1`).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("fake-cookie-v1")) {
		t.Fatal("plaintext stored")
	}
	if err := store.Rollback(context.Background(), "youtube", DefaultProfile, first.Revision); err != nil {
		t.Fatal(err)
	}
	current, err := store.Current(context.Background(), "youtube", DefaultProfile)
	if err != nil || current.Cookies[0].Value != "fake-cookie-v1" {
		t.Fatalf("rollback failed: %v", err)
	}
}
