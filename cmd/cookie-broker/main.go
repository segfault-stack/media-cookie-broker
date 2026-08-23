package main

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/segfault-stack/media-cookie-broker/internal/broker"
)

func main() {
	syscall.Umask(0o077)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	addr := env("BROKER_LISTEN_ADDR", "127.0.0.1:8787")
	dbPath := env("BROKER_DB_PATH", "/data/broker.sqlite3")
	key, err := loadKey(env("BROKER_MASTER_KEY_FILE", "/run/secrets/master-key"))
	fatal(logger, err)
	store, err := broker.OpenStore(dbPath, key)
	fatal(logger, err)
	defer store.Close()
	server := &http.Server{Addr: addr, Handler: broker.NewHandler(store, broker.NewAuth(store), logger), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	logger.Info("cookie broker listening", "address", addr)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	serverError := make(chan error, 1)
	go func() { serverError <- server.ListenAndServe() }()
	select {
	case err = <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			fatal(logger, err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err = server.Shutdown(shutdownCtx); err != nil {
			fatal(logger, err)
		}
		if err = <-serverError; !errors.Is(err, http.ErrServerClosed) {
			fatal(logger, err)
		}
	}
}

func loadKey(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return base64.RawStdEncoding.DecodeString(string(bytesTrim(raw)))
}
func bytesTrim(value []byte) []byte {
	for len(value) > 0 && (value[len(value)-1] == '\n' || value[len(value)-1] == '\r' || value[len(value)-1] == ' ') {
		value = value[:len(value)-1]
	}
	return value
}
func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func fatal(logger *slog.Logger, err error) {
	if err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}
