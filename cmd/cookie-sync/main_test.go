package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTargets(t *testing.T) {
	targets, err := parseTargets("youtube=/cookies/youtube.txt,x=/cookies/x.txt")
	if err != nil || len(targets) != 2 || targets[1].provider != "x" {
		t.Fatalf("unexpected targets: %#v, %v", targets, err)
	}
	if _, err := parseTargets("youtube=relative.txt"); err == nil {
		t.Fatal("relative output path must be rejected")
	}
	if targets, err := parseTargets("future-provider=/cookies/future.txt"); err != nil || targets[0].provider != "future-provider" {
		t.Fatalf("syntactically valid future provider was rejected: %#v, %v", targets, err)
	}
	for _, value := range []string{"Uppercase=/cookies/invalid.txt", "with.dot=/cookies/invalid.txt", "-leading=/cookies/invalid.txt", "provider-name-that-is-longer-than-32=/cookies/invalid.txt"} {
		if _, err := parseTargets(value); err == nil {
			t.Fatalf("invalid provider ID accepted: %s", value)
		}
	}
	if _, err := parseTargets("youtube=/cookies/same.txt,x=/cookies/same.txt"); err == nil {
		t.Fatal("duplicate output path must be rejected")
	}
	if _, err := parseTargets("youtube=/cookies/youtube.txt,x=/cookies/youtube.txt.meta.json"); err == nil {
		t.Fatal("target/sidecar path collision must be rejected")
	}
	profiles, err := parseTargets("youtube/default=/cookies/default.txt,youtube/music-bot=/cookies/music.txt")
	if err != nil || profiles[1].profile != "music-bot" {
		t.Fatalf("named profile target failed: %#v %v", profiles, err)
	}
	if _, err := parseTargets("youtube/../private=/cookies/private.txt"); err == nil {
		t.Fatal("path traversal profile accepted")
	}
}

func TestFetchWritesPrivateRevisionMetadataSidecar(t *testing.T) {
	cookieData := []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\tvalue\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Cookie-Revision", "17")
		w.Header().Set("X-Cookie-Captured-At", "2026-08-23T10:00:00Z")
		w.Header().Set("X-Cookie-Created-At", "2026-08-23T10:00:01Z")
		w.Header().Set("X-Cookie-Provider", "youtube")
		w.Header().Set("X-Cookie-Profile", "music-bot")
		_, _ = w.Write(cookieData)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "youtube.txt")
	target := &target{provider: "youtube", profile: "music-bot", path: path, metadata: true}
	if err := fetch(context.Background(), server.Client(), server.URL, "reader", "not-a-real-password", target); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(metadataPath(path))
	if err != nil {
		t.Fatal(err)
	}
	var metadata cookieMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(cookieData)
	if metadata.Provider != "youtube" || metadata.Profile != "music-bot" || metadata.Revision != 17 || metadata.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	if bytes.Contains(raw, []byte("value")) || bytes.Contains(raw, []byte("password")) {
		t.Fatal("sidecar contains secret material")
	}
	if info, err := os.Stat(metadataPath(path)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected sidecar mode: %v %v", info, err)
	}
}

func TestReportUsesSidecarRevisionAndFailsClosedOnMismatch(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/providers/youtube/profiles/music-bot/reports" {
			t.Fatalf("wrong report path: %s", r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "reader" || password != "not-a-real-password" {
			t.Fatal("missing report authentication")
		}
		var report struct {
			Revision int64  `json:"revision"`
			Kind     string `json:"kind"`
		}
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Fatal(err)
		}
		if report.Revision != 17 || report.Kind != "authentication_required" {
			t.Fatalf("wrong report: %#v", report)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	dir := t.TempDir()
	cookiePath := filepath.Join(dir, "youtube.txt")
	cookieData := []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\tfake\n")
	if err := os.WriteFile(cookiePath, cookieData, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(cookieData)
	metadata := cookieMetadata{SchemaVersion: 1, Provider: "youtube", Profile: "music-bot", Revision: 17, SHA256: hex.EncodeToString(sum[:])}
	raw, _ := json.Marshal(metadata)
	sidecar := metadataPath(cookiePath)
	if err := os.WriteFile(sidecar, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	revision, err := reportHealth(context.Background(), server.Client(), server.URL, "reader", "not-a-real-password", "youtube", "music-bot", cookiePath, sidecar, "authentication_required")
	if err != nil || revision != 17 || requests != 1 {
		t.Fatalf("report failed: revision=%d requests=%d err=%v", revision, requests, err)
	}
	if err := os.WriteFile(cookiePath, append(cookieData, []byte("# changed\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reportHealth(context.Background(), server.Client(), server.URL, "reader", "not-a-real-password", "youtube", "music-bot", cookiePath, sidecar, "authentication_required"); err == nil {
		t.Fatal("SHA mismatch did not fail closed")
	}
	if requests != 1 {
		t.Fatal("mismatched sidecar sent a report")
	}
}

func TestBrokerURLAndCombinedPathValidation(t *testing.T) {
	if value, err := parseBaseURL("https://cookies.example/"); err != nil || value != "https://cookies.example" {
		t.Fatalf("unexpected HTTPS URL: %q %v", value, err)
	}
	if _, err := parseBaseURL("http://127.0.0.1:8787"); err != nil {
		t.Fatalf("loopback HTTP must be allowed: %v", err)
	}
	for _, value := range []string{"http://cookies.example", "https://user:pass@cookies.example", "https://cookies.example?secret=x"} {
		if _, err := parseBaseURL(value); err == nil {
			t.Fatalf("unsafe broker URL accepted: %s", value)
		}
	}
	targets, _ := parseTargets("youtube=/cookies/youtube.txt")
	if err := validateCombined("/cookies/youtube.txt", targets); err == nil {
		t.Fatal("combined output must not replace a provider output")
	}
}

func TestFetchUsesETagAndRetainsLastGoodFile(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") == "" {
			t.Fatal("missing Basic authorization")
		}
		w.Header().Set("X-Cookie-Revision", "1")
		w.Header().Set("X-Cookie-Provider", "youtube")
		w.Header().Set("X-Cookie-Profile", "default")
		if r.Header.Get("If-None-Match") == `"hash"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"hash"`)
		_, _ = w.Write([]byte("# Netscape HTTP Cookie File\n# test\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\tvalue\n"))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "youtube.txt")
	target := &target{provider: "youtube", profile: "default", path: path, metadata: true}
	if err := fetch(context.Background(), server.Client(), server.URL, "reader", "password", target); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected mode: %v, %v", info, err)
	}
	if err := fetch(context.Background(), server.Client(), server.URL, "reader", "password", target); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) || requests != 2 {
		t.Fatalf("conditional fetch changed file or request count: %d", requests)
	}
	if _, err := os.Stat(metadataPath(path)); err != nil {
		t.Fatalf("conditional sync did not preserve metadata sidecar: %v", err)
	}
}

func TestNotModifiedRestoresTamperedCookieFileBeforeReporting(t *testing.T) {
	original := []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\tfake-original\n")
	var getHeaders []string
	reports := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			reports++
			var report struct {
				Revision int64  `json:"revision"`
				Kind     string `json:"kind"`
			}
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Fatal(err)
			}
			if report.Revision != 1 || report.Kind != "authentication_required" {
				t.Fatalf("unexpected report: %#v", report)
			}
			w.WriteHeader(http.StatusCreated)
			return
		}
		conditional := r.Header.Get("If-None-Match")
		getHeaders = append(getHeaders, conditional)
		if conditional != "" {
			if conditional != `"revision-1"` {
				t.Fatalf("unexpected conditional ETag: %q", conditional)
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"revision-1"`)
		w.Header().Set("X-Cookie-Revision", "1")
		w.Header().Set("X-Cookie-Provider", "youtube")
		w.Header().Set("X-Cookie-Profile", "default")
		_, _ = w.Write(original)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "youtube.txt")
	target := &target{provider: "youtube", profile: "default", path: path, metadata: true}
	if err := fetch(context.Background(), server.Client(), server.URL, "reader", "not-a-real-password", target); err != nil {
		t.Fatal(err)
	}
	assertMetadata := func() {
		t.Helper()
		raw, err := os.ReadFile(metadataPath(path))
		if err != nil {
			t.Fatal(err)
		}
		var metadata cookieMetadata
		if err := json.Unmarshal(raw, &metadata); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(original)
		if metadata.Revision != 1 || metadata.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("metadata no longer describes broker revision 1: %#v", metadata)
		}
	}
	assertMetadata()

	tampered := append(append([]byte(nil), original...), []byte("# tampered\n")...)
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fetch(context.Background(), server.Client(), server.URL, "reader", "not-a-real-password", target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) || !bytes.Equal(target.content, original) {
		t.Fatal("304 recovery did not restore broker-backed cookie content")
	}
	assertMetadata()
	if len(getHeaders) != 3 || getHeaders[0] != "" || getHeaders[1] != `"revision-1"` || getHeaders[2] != "" {
		t.Fatalf("expected full, conditional, then unconditional GET; got %#v", getHeaders)
	}

	revision, err := reportHealth(context.Background(), server.Client(), server.URL, "reader", "not-a-real-password", "youtube", "default", path, metadataPath(path), "authentication_required")
	if err != nil || revision != 1 || reports != 1 {
		t.Fatalf("report after restoration: revision=%d reports=%d err=%v", revision, reports, err)
	}
}

func TestNotModifiedRefetchesWhenTrustedSidecarIsMissingOrCorrupt(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		change func(string) error
	}{
		{name: "missing", change: os.Remove},
		{name: "corrupt", change: func(path string) error { return os.WriteFile(path, []byte("not-json\n"), 0o600) }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			original := []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\tfake-original\n")
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Header.Get("If-None-Match") != "" {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				w.Header().Set("ETag", `"revision-1"`)
				w.Header().Set("X-Cookie-Revision", "1")
				w.Header().Set("X-Cookie-Provider", "youtube")
				w.Header().Set("X-Cookie-Profile", "default")
				_, _ = w.Write(original)
			}))
			defer server.Close()

			path := filepath.Join(t.TempDir(), "youtube.txt")
			target := &target{provider: "youtube", profile: "default", path: path, metadata: true}
			if err := fetch(context.Background(), server.Client(), server.URL, "reader", "password", target); err != nil {
				t.Fatal(err)
			}
			if err := testCase.change(metadataPath(path)); err != nil {
				t.Fatal(err)
			}
			if err := fetch(context.Background(), server.Client(), server.URL, "reader", "password", target); err != nil {
				t.Fatal(err)
			}
			if requests != 3 {
				t.Fatalf("sidecar %s did not force an unconditional fetch: %d requests", testCase.name, requests)
			}
			raw, err := os.ReadFile(metadataPath(path))
			if err != nil {
				t.Fatal(err)
			}
			var metadata cookieMetadata
			if err := json.Unmarshal(raw, &metadata); err != nil || metadata.Revision != 1 {
				t.Fatalf("sidecar was not restored from a 200 response: %#v, %v", metadata, err)
			}
		})
	}
}

func TestNotModifiedNeverPoisonsMetadataDisabledLastGoodContent(t *testing.T) {
	original := []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\tfake-original\n")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("ETag", `"revision-1"`)
			_, _ = w.Write(original)
			return
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "youtube.txt")
	target := &target{provider: "youtube", profile: "default", path: path, metadata: false}
	if err := fetch(context.Background(), server.Client(), server.URL, "reader", "password", target); err != nil {
		t.Fatal(err)
	}
	tampered := append(append([]byte(nil), original...), []byte("# tampered\n")...)
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fetch(context.Background(), server.Client(), server.URL, "reader", "password", target); err == nil {
		t.Fatal("304 response to the unconditional recovery request was accepted")
	}
	if requests != 3 {
		t.Fatalf("expected one bounded unconditional retry, got %d requests", requests)
	}
	if !bytes.Equal(target.content, original) || !bytes.Equal(lastGoodContent(target), original) {
		t.Fatal("locally modified bytes poisoned metadata-disabled last-good content")
	}
}

func TestInvalidResponseDoesNotReplaceFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not a cookie jar"))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "youtube.txt")
	if err := os.WriteFile(path, []byte("last-good"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := &target{provider: "youtube", path: path}
	if err := fetch(context.Background(), server.Client(), server.URL, "reader", "password", target); err == nil {
		t.Fatal("expected invalid response")
	}
	content, _ := os.ReadFile(path)
	if string(content) != "last-good" {
		t.Fatal("last good file was replaced")
	}
}

func TestMissingLocalFileAfterNotModifiedIsFetchedAgain(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("If-None-Match") != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"new"`)
		_, _ = w.Write([]byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\tvalue\n"))
	}))
	defer server.Close()
	target := &target{provider: "youtube", path: filepath.Join(t.TempDir(), "youtube.txt"), etag: `"old"`}

	if err := fetch(context.Background(), server.Client(), server.URL, "reader", "password", target); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("expected conditional request plus full recovery, got %d", requests)
	}
	if _, err := os.Stat(target.path); err != nil {
		t.Fatal(err)
	}
}

func TestOversizedResponseDoesNotReplaceLastGoodFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(append([]byte("# Netscape HTTP Cookie File\n"), bytes.Repeat([]byte("x"), (2<<20)+1)...))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "youtube.txt")
	if err := os.WriteFile(path, []byte("last-good"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fetch(context.Background(), server.Client(), server.URL, "reader", "password", &target{provider: "youtube", path: path}); err == nil {
		t.Fatal("expected oversized response")
	}
	content, _ := os.ReadFile(path)
	if string(content) != "last-good" {
		t.Fatal("oversized response replaced last good file")
	}
}

func TestSyncAllWritesCombinedFromAvailableAndLastGoodProviders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) != "cookies.txt" {
			http.NotFound(w, r)
			return
		}
		if filepath.Base(filepath.Dir(r.URL.Path)) == "youtube" {
			_, _ = w.Write([]byte("# Netscape HTTP Cookie File\n# youtube\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\tnew\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	dir := t.TempDir()
	tiktokPath := filepath.Join(dir, "tiktok.txt")
	if err := os.WriteFile(tiktokPath, []byte("# Netscape HTTP Cookie File\n# tiktok\n.tiktok.com\tTRUE\t/\tTRUE\t0\tsid\tlast-good\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	targets := []*target{
		{provider: "youtube", path: filepath.Join(dir, "youtube.txt")},
		{provider: "tiktok", path: tiktokPath},
	}
	combined := filepath.Join(dir, "combined.txt")
	if err := syncAll(context.Background(), server.Client(), server.URL, "reader", "password", targets, combined); err == nil {
		t.Fatal("expected missing tiktok snapshot to remain visible as an error")
	}
	data, err := os.ReadFile(combined)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "SID\tnew") || !strings.Contains(content, "sid\tlast-good") {
		t.Fatalf("combined file did not preserve available providers: %q", content)
	}
}
