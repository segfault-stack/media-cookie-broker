package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/segfault-dev/media-cookie-broker/internal/broker"
)

func TestPasswordInputAndGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("correct horse battery staple\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	password, generated, err := passwordOrGenerate(path)
	if err != nil || generated || password != "correct horse battery staple" {
		t.Fatalf("password file: %q %v %v", password, generated, err)
	}
	password, generated, err = passwordOrGenerate("")
	if err != nil || !generated || len(password) < 20 {
		t.Fatalf("generated password: length=%d generated=%v err=%v", len(password), generated, err)
	}
	if _, err := broker.HashPassword(password); err != nil {
		t.Fatalf("generated password is not accepted: %v", err)
	}
}
