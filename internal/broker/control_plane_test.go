package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDatabaseUsersAndProfileScopedAuthorization(t *testing.T) {
	_, store := testAPI(t)
	ctx := context.Background()
	if err := store.CreateUser(ctx, "musicbot", "another correct horse battery", "reader", []Scope{{Provider: "youtube", Profile: DefaultProfile}}); err != nil {
		t.Fatal(err)
	}
	auth := NewAuth(store)
	if !auth.Authenticate(ctx, "musicbot", "another correct horse battery", "reader", "youtube", DefaultProfile) {
		t.Fatal("default profile grant did not authenticate")
	}
	if auth.Authenticate(ctx, "musicbot", "another correct horse battery", "reader", "youtube", "music-bot") {
		t.Fatal("default profile grant leaked into named profile")
	}
	if err := store.Grant(ctx, "musicbot", "youtube", "music-bot"); err != nil {
		t.Fatal(err)
	}
	if !auth.Authenticate(ctx, "musicbot", "another correct horse battery", "reader", "youtube", "music-bot") {
		t.Fatal("runtime grant did not take effect")
	}
	if err := store.Revoke(ctx, "musicbot", "youtube", "music-bot"); err != nil {
		t.Fatal(err)
	}
	if auth.Authenticate(ctx, "musicbot", "another correct horse battery", "reader", "youtube", "music-bot") {
		t.Fatal("runtime revoke did not take effect")
	}
	if err := store.ChangePassword(ctx, "musicbot", "replacement correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if auth.Authenticate(ctx, "musicbot", "another correct horse battery", "reader", "youtube", DefaultProfile) ||
		!auth.Authenticate(ctx, "musicbot", "replacement correct horse battery", "reader", "youtube", DefaultProfile) {
		t.Fatal("password change was not reflected at runtime")
	}
	users, err := store.ListUsers(ctx)
	if err != nil || len(users) != 4 {
		t.Fatalf("list users: %d, %v", len(users), err)
	}
	if err := store.CreateUser(ctx, "musicbot", "another correct horse battery", "reader", nil); err == nil {
		t.Fatal("duplicate username accepted")
	}
	if err := store.CreateUser(ctx, "bad/name", "another correct horse battery", "reader", nil); err == nil {
		t.Fatal("invalid username accepted")
	}
	if err := store.CreateUser(ctx, "badrole", "another correct horse battery", "admin", nil); err == nil {
		t.Fatal("invalid role accepted")
	}
	unsafeHash := "$argon2id$v=19$m=999999,t=3,p=2$MDEyMzQ1Njc4OWFiY2RlZg$MDEyMzQ1Njc4OWFiY2RlZg"
	if err := store.createUserWithHash(ctx, "unsafe", unsafeHash, "reader", nil); err == nil {
		t.Fatal("unsafe Argon2 parameters accepted")
	}
	if err := store.Grant(ctx, "musicbot", "unknown", DefaultProfile); err == nil {
		t.Fatal("invalid provider accepted")
	}
	if err := store.Grant(ctx, "musicbot", "youtube", "../private"); err == nil {
		t.Fatal("path traversal profile accepted")
	}
	if err := store.DeleteUser(ctx, "musicbot"); err != nil {
		t.Fatal(err)
	}
	if auth.Authenticate(ctx, "musicbot", "replacement correct horse battery", "reader", "youtube", DefaultProfile) {
		t.Fatal("deleted user still authenticates")
	}
}

func TestProfileRoutesKeepIndependentRevisionsAndACLs(t *testing.T) {
	h, store := testAPI(t)
	ctx := context.Background()
	if err := store.Grant(ctx, "writer", "youtube", "music-bot"); err != nil {
		t.Fatal(err)
	}
	if err := store.Grant(ctx, "reader", "youtube", "music-bot"); err != nil {
		t.Fatal(err)
	}
	putSnapshot(t, h, "/v1/providers/youtube/cookies", "default-cookie", time.Time{})
	putSnapshot(t, h, "/v1/providers/youtube/profiles/music-bot/cookies", "music-cookie", time.Time{})
	for path, wanted := range map[string]string{
		"/v1/providers/youtube/cookies.txt":                    "default-cookie",
		"/v1/providers/youtube/profiles/music-bot/cookies.txt": "music-cookie",
	} {
		response := authenticatedRequest(h, http.MethodGet, path, nil, "reader")
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(wanted)) {
			t.Fatalf("GET %s = %d %s", path, response.Code, response.Body.String())
		}
	}
	defaultSnapshot, _ := store.Current(ctx, "youtube", DefaultProfile)
	musicSnapshot, _ := store.Current(ctx, "youtube", "music-bot")
	if defaultSnapshot.Status.Revision != 1 || musicSnapshot.Status.Revision != 1 {
		t.Fatalf("revisions are not independently scoped: %#v %#v", defaultSnapshot.Status, musicSnapshot.Status)
	}
	putSnapshot(t, h, "/v1/providers/youtube/profiles/music-bot/cookies", "music-cookie-v2", time.Time{})
	if err := store.Rollback(ctx, "youtube", "music-bot", 1); err != nil {
		t.Fatal(err)
	}
	defaultSnapshot, _ = store.Current(ctx, "youtube", DefaultProfile)
	musicSnapshot, _ = store.Current(ctx, "youtube", "music-bot")
	if defaultSnapshot.Cookies[0].Value != "default-cookie" || musicSnapshot.Cookies[0].Value != "music-cookie" {
		t.Fatalf("profile rollback crossed scope: default=%q music=%q", defaultSnapshot.Cookies[0].Value, musicSnapshot.Cookies[0].Value)
	}
	if err := store.Rollback(ctx, "youtube", "music-bot", 99); err == nil {
		t.Fatal("rollback crossed or accepted missing named-profile revision")
	}
	response := authenticatedRequest(h, http.MethodGet, "/v1/status", nil, "writer")
	if response.Code != http.StatusOK {
		t.Fatalf("publisher profile list = %d %s", response.Code, response.Body.String())
	}
	var listed struct {
		Profiles []Status `json:"profiles"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil || len(listed.Profiles) != 2 {
		t.Fatalf("publisher-visible profiles = %#v %v", listed.Profiles, err)
	}
	response = authenticatedRequest(h, http.MethodGet, "/v1/providers/youtube/profiles/private-account/status", nil, "reader")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized profile status = %d", response.Code)
	}
}

func TestConsumerReportsAreRevisionAndIdentityScoped(t *testing.T) {
	h, store := testAPI(t)
	ctx := context.Background()
	if err := store.CreateUser(ctx, "relay", "relay correct horse battery", "reader", []Scope{{Provider: "youtube", Profile: DefaultProfile}}); err != nil {
		t.Fatal(err)
	}
	putSnapshot(t, h, "/v1/providers/youtube/cookies", "revision-one", time.Time{})
	postReport(t, h, "reader", 1, "authentication_required", http.StatusOK)
	status := getStatus(t, h, "/v1/providers/youtube/status")
	if status.AuthHealth != "refresh_required" || status.AuthRequiredCount != 1 {
		t.Fatalf("authentication report did not request refresh: %#v", status)
	}
	postReport(t, h, "relay", 1, "ok", http.StatusOK)
	if status = getStatus(t, h, "/v1/providers/youtube/status"); status.AuthHealth != "refresh_required" {
		t.Fatal("another consumer's OK cleared the failure")
	}
	postReport(t, h, "reader", 1, "ok", http.StatusOK)
	if status = getStatus(t, h, "/v1/providers/youtube/status"); status.AuthHealth != "healthy" {
		t.Fatal("same consumer's OK did not supersede its failure")
	}
	postReport(t, h, "reader", 1, "rate_limited", http.StatusOK)
	if status = getStatus(t, h, "/v1/providers/youtube/status"); status.AuthHealth != "healthy" {
		t.Fatal("non-authentication failure requested refresh")
	}
	postReport(t, h, "reader", 1, "authentication_required", http.StatusOK)
	putSnapshot(t, h, "/v1/providers/youtube/cookies", "revision-two", time.Time{})
	if status = getStatus(t, h, "/v1/providers/youtube/status"); status.Revision != 2 || status.AuthHealth != "healthy" {
		t.Fatalf("old report poisoned new revision: %#v", status)
	}
	postReport(t, h, "reader", 1, "authentication_required", http.StatusOK)
	if status = getStatus(t, h, "/v1/providers/youtube/status"); status.AuthHealth != "healthy" {
		t.Fatal("later historical report poisoned current revision")
	}
	response := authenticatedRequest(h, http.MethodPost, "/v1/providers/youtube/reports", bytes.NewBufferString(`{"revision":2,"kind":"authentication_required","consumer_id":"other"}`), "reader")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("caller-supplied consumer identity accepted: %d", response.Code)
	}
	response = authenticatedRequest(h, http.MethodPost, "/v1/providers/youtube/profiles/private-account/reports", bytes.NewBufferString(`{"revision":2,"kind":"authentication_required"}`), "reader")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("reader reported unauthorized profile: %d", response.Code)
	}
	response = authenticatedRequest(h, http.MethodGet, "/v1/providers/youtube/cookies.txt", nil, "reader")
	if response.Code != http.StatusOK {
		t.Fatal(response.Code)
	}
	etag := response.Header().Get("ETag")
	request := httptest.NewRequest(http.MethodGet, "/v1/providers/youtube/cookies.txt", nil)
	request.SetBasicAuth("reader", "correct horse battery staple")
	request.Header.Set("If-None-Match", etag)
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotModified {
		t.Fatal(recorder.Code)
	}
	seen, err := store.ConsumerState(ctx, "reader", "youtube", DefaultProfile)
	if err != nil || seen.RevisionSeen != 2 || seen.LastSeen.IsZero() {
		t.Fatalf("consumer activity not recorded: %#v %v", seen, err)
	}
}

func TestRecoveryPublicationAlwaysAdvancesAuthenticationLifecycle(t *testing.T) {
	h, store := testAPI(t)
	publish := func(value string, reason PublicationReason) (Status, int) {
		t.Helper()
		upload := Upload{
			SchemaVersion:     1,
			PublicationReason: reason,
			CapturedAt:        time.Now().UTC(),
			Cookies:           []Cookie{{Domain: ".youtube.com", Path: "/", Name: "SID", Value: value}},
		}
		body, _ := json.Marshal(upload)
		response := authenticatedRequest(h, http.MethodPut, "/v1/providers/youtube/cookies", bytes.NewReader(body), "writer")
		var status Status
		if response.Code == http.StatusOK || response.Code == http.StatusCreated {
			if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
		}
		return status, response.Code
	}

	first, code := publish("same-cookie-material", PublicationOrdinary)
	if code != http.StatusCreated || first.Revision != 1 || first.PublicationReason != PublicationOrdinary {
		t.Fatalf("first ordinary publication = %d %#v", code, first)
	}
	duplicate, code := publish("same-cookie-material", PublicationOrdinary)
	if code != http.StatusOK || duplicate.Revision != 1 || duplicate.Changed {
		t.Fatalf("ordinary duplicate was not deduplicated = %d %#v", code, duplicate)
	}
	postReport(t, h, "reader", 1, "authentication_required", http.StatusOK)
	if status := getStatus(t, h, "/v1/providers/youtube/status"); status.AuthHealth != "refresh_required" {
		t.Fatalf("pre-recovery failure is not active: %#v", status)
	}

	recovered, code := publish("same-cookie-material", PublicationRecovery)
	if code != http.StatusCreated || recovered.Revision != 2 || !recovered.Changed || recovered.PublicationReason != PublicationRecovery {
		t.Fatalf("identical recovery did not advance exactly once = %d %#v", code, recovered)
	}
	if status := getStatus(t, h, "/v1/providers/youtube/status"); status.Revision != 2 || status.AuthHealth != "healthy" {
		t.Fatalf("old failure affected recovery revision: %#v", status)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/providers/youtube/cookies.txt", nil)
	request.SetBasicAuth("reader", "correct horse battery staple")
	request.Header.Set("If-None-Match", `"`+recovered.SHA256+`"`)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusNotModified || response.Header().Get("X-Cookie-Revision") != "2" {
		t.Fatalf("identical recovery did not retain ETag semantics with new revision metadata: %d %#v", response.Code, response.Header())
	}
	var historical int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM consumer_reports WHERE provider='youtube' AND profile='default' AND revision=1 AND kind='authentication_required'`).Scan(&historical); err != nil || historical != 1 {
		t.Fatalf("old recovery report history was not preserved: count=%d err=%v", historical, err)
	}

	changed, code := publish("changed-cookie-material", PublicationRecovery)
	if code != http.StatusCreated || changed.Revision != 3 {
		t.Fatalf("changed recovery did not advance exactly once = %d %#v", code, changed)
	}
	failed := Upload{SchemaVersion: 1, PublicationReason: PublicationRecovery, CapturedAt: time.Now().UTC(), Cookies: []Cookie{{Domain: ".youtube.com", Path: "/bad\nrow", Name: "SID", Value: "not-a-real-cookie"}}}
	body, _ := json.Marshal(failed)
	response = authenticatedRequest(h, http.MethodPut, "/v1/providers/youtube/cookies", bytes.NewReader(body), "writer")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid recovery upload = %d %s", response.Code, response.Body.String())
	}
	if status := getStatus(t, h, "/v1/providers/youtube/status"); status.Revision != 3 {
		t.Fatalf("failed recovery advanced revision: %#v", status)
	}
	ordinaryAfterRecovery, code := publish("changed-cookie-material", PublicationOrdinary)
	if code != http.StatusOK || ordinaryAfterRecovery.Revision != 3 || ordinaryAfterRecovery.Changed {
		t.Fatalf("ordinary duplicate after recovery did not deduplicate = %d %#v", code, ordinaryAfterRecovery)
	}
}

func TestRevokedConsumerReportsRemainHistoricalButBecomeInactive(t *testing.T) {
	h, store := testAPI(t)
	ctx := context.Background()
	if err := store.CreateUser(ctx, "relay", "relay correct horse battery", "reader", []Scope{{Provider: "youtube", Profile: DefaultProfile}}); err != nil {
		t.Fatal(err)
	}
	for _, username := range []string{"writer", "reader"} {
		if err := store.Grant(ctx, username, "youtube", "music-bot"); err != nil {
			t.Fatal(err)
		}
	}
	putSnapshot(t, h, "/v1/providers/youtube/cookies", "default-revision", time.Time{})
	putSnapshot(t, h, "/v1/providers/youtube/profiles/music-bot/cookies", "music-revision", time.Time{})
	postReport(t, h, "reader", 1, "authentication_required", http.StatusOK)
	postReport(t, h, "relay", 1, "authentication_required", http.StatusOK)
	if err := store.PutHealthReport(ctx, "reader", "youtube", "music-bot", HealthReport{Revision: 1, Kind: "authentication_required"}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordConsumerSeen(ctx, "reader", "youtube", DefaultProfile, 1); err != nil {
		t.Fatal(err)
	}

	if err := store.Revoke(ctx, "reader", "youtube", DefaultProfile); err != nil {
		t.Fatal(err)
	}
	status := getStatus(t, h, "/v1/providers/youtube/status")
	if status.AuthHealth != "refresh_required" || status.AuthRequiredCount != 1 {
		t.Fatalf("another authorized consumer report did not remain active: %#v", status)
	}
	musicStatus := getStatus(t, h, "/v1/providers/youtube/profiles/music-bot/status")
	if musicStatus.AuthHealth != "refresh_required" || musicStatus.AuthRequiredCount != 1 {
		t.Fatalf("exact-scope revoke affected another profile: %#v", musicStatus)
	}
	var historical int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM consumer_reports WHERE username='reader' AND provider='youtube' AND profile='default' AND kind='authentication_required'`).Scan(&historical); err != nil || historical != 1 {
		t.Fatalf("revoked report history was lost: count=%d err=%v", historical, err)
	}
	if _, err := store.ConsumerState(ctx, "reader", "youtube", DefaultProfile); err != nil {
		t.Fatalf("revocation removed consumer activity history: %v", err)
	}
	response := authenticatedRequest(h, http.MethodGet, "/v1/providers/youtube/cookies.txt", nil, "reader")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked reader still fetched cookies: %d", response.Code)
	}
	postReport(t, h, "reader", 1, "ok", http.StatusUnauthorized)

	if err := store.Revoke(ctx, "relay", "youtube", DefaultProfile); err != nil {
		t.Fatal(err)
	}
	status = getStatus(t, h, "/v1/providers/youtube/status")
	if status.AuthHealth != "healthy" || status.AuthRequiredCount != 0 || status.LastReportAt != nil {
		t.Fatalf("revoked reports still affected active health: %#v", status)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM consumer_reports WHERE provider='youtube' AND profile='default'`).Scan(&historical); err != nil || historical != 2 {
		t.Fatalf("historical reports were not preserved: count=%d err=%v", historical, err)
	}

	if err := store.Grant(ctx, "reader", "youtube", DefaultProfile); err != nil {
		t.Fatal(err)
	}
	status = getStatus(t, h, "/v1/providers/youtube/status")
	if status.AuthHealth != "healthy" || status.AuthRequiredCount != 0 {
		t.Fatalf("re-grant resurrected stale failure: %#v", status)
	}
	postReport(t, h, "reader", 1, "authentication_required", http.StatusOK)
	status = getStatus(t, h, "/v1/providers/youtube/status")
	if status.AuthHealth != "refresh_required" || status.AuthRequiredCount != 1 {
		t.Fatalf("fresh post-grant report did not become active: %#v", status)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE users SET role='publisher' WHERE username='reader'`); err != nil {
		t.Fatal(err)
	}
	status = getStatus(t, h, "/v1/providers/youtube/status")
	if status.AuthHealth != "healthy" || status.AuthRequiredCount != 0 {
		t.Fatalf("non-reader report still affected active health: %#v", status)
	}
}

func TestProviderAuthExpiryUsesOnlyRelevantCookies(t *testing.T) {
	h, _ := testAPI(t)
	authExpiry := time.Now().UTC().Add(12 * time.Hour).Truncate(time.Second)
	putSnapshot(t, h, "/v1/providers/youtube/cookies", "auth-cookie", authExpiry)
	status := getStatus(t, h, "/v1/providers/youtube/status")
	if status.AuthExpiresAt == nil || !status.AuthExpiresAt.Equal(authExpiry) || status.AuthExpirySource != "provider_auth_cookie" {
		t.Fatalf("wrong auth expiry hint: %#v", status)
	}
	upload := Upload{SchemaVersion: 1, CapturedAt: time.Now().UTC(), Cookies: []Cookie{{Domain: ".youtube.com", Path: "/", Name: "unrelated", Value: "fake", Expiration: time.Now().Add(time.Minute).Unix()}}}
	body, _ := json.Marshal(upload)
	response := authenticatedRequest(h, http.MethodPut, "/v1/providers/youtube/cookies", bytes.NewReader(body), "writer")
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code)
	}
	status = getStatus(t, h, "/v1/providers/youtube/status")
	if status.AuthExpiresAt != nil {
		t.Fatalf("fabricated expiry from unrelated cookie: %#v", status)
	}
}

func putSnapshot(t *testing.T, handler http.Handler, path, value string, authExpiry time.Time) {
	t.Helper()
	cookies := []Cookie{{Domain: ".youtube.com", Path: "/", Name: "unrelated", Value: value, Expiration: time.Now().Add(time.Hour).Unix()}}
	if !authExpiry.IsZero() {
		cookies = append(cookies, Cookie{Domain: ".youtube.com", Path: "/", Name: "SID", Value: value + "-sid", Expiration: authExpiry.Unix()})
	}
	upload := Upload{SchemaVersion: 1, CapturedAt: time.Now().UTC(), Cookies: cookies}
	body, _ := json.Marshal(upload)
	response := authenticatedRequest(handler, http.MethodPut, path, bytes.NewReader(body), "writer")
	if response.Code != http.StatusCreated {
		t.Fatalf("publish %s = %d %s", path, response.Code, response.Body.String())
	}
}

func postReport(t *testing.T, handler http.Handler, username string, revision int64, kind string, wanted int) {
	t.Helper()
	body, _ := json.Marshal(HealthReport{Revision: revision, Kind: kind})
	response := authenticatedRequest(handler, http.MethodPost, "/v1/providers/youtube/reports", bytes.NewReader(body), username)
	if response.Code != wanted {
		t.Fatalf("report %s = %d %s", kind, response.Code, response.Body.String())
	}
}

func getStatus(t *testing.T, handler http.Handler, path string) Status {
	t.Helper()
	response := authenticatedRequest(handler, http.MethodGet, path, nil, "writer")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d %s", response.Code, response.Body.String())
	}
	var status Status
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func authenticatedRequest(handler http.Handler, method, path string, body io.Reader, username string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	password := "correct horse battery staple"
	if username == "relay" {
		password = "relay correct horse battery"
	}
	request.SetBasicAuth(username, password)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
