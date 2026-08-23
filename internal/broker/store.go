package broker

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	aead cipher.AEAD
}

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func OpenStore(path string, key []byte) (*Store, error) {
	if len(key) != 32 {
		return nil, errors.New("master key must be exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, aead: aead}
	if err := s.initSchema(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if path != ":memory:" {
		if err := os.Chmod(path, 0o600); err != nil {
			db.Close()
			return nil, fmt.Errorf("set database permissions: %w", err)
		}
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) initSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS revisions (
 provider TEXT NOT NULL, profile TEXT NOT NULL, revision INTEGER NOT NULL, created_at TEXT NOT NULL,
	 captured_at TEXT NOT NULL, sha256 TEXT NOT NULL, cookie_count INTEGER NOT NULL,
	 publication_reason TEXT NOT NULL CHECK(publication_reason IN ('ordinary','recovery')),
	 auth_expires_at TEXT, auth_expiry_source TEXT NOT NULL DEFAULT '',
 nonce BLOB NOT NULL, ciphertext BLOB NOT NULL, PRIMARY KEY(provider, profile, revision));
CREATE TABLE IF NOT EXISTS current_revisions (
 provider TEXT NOT NULL, profile TEXT NOT NULL, revision INTEGER NOT NULL,
 PRIMARY KEY(provider, profile),
 FOREIGN KEY(provider, profile, revision) REFERENCES revisions(provider, profile, revision));
CREATE TABLE IF NOT EXISTS diagnostics (
 id INTEGER PRIMARY KEY AUTOINCREMENT, provider TEXT NOT NULL, profile TEXT NOT NULL DEFAULT 'default',
 installation_id TEXT NOT NULL, timestamp TEXT NOT NULL, severity TEXT NOT NULL, event_type TEXT NOT NULL,
 nonce BLOB NOT NULL, ciphertext BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS users (
 username TEXT PRIMARY KEY, password_hash TEXT NOT NULL,
 role TEXT NOT NULL CHECK(role IN ('publisher','reader','diagnostics_reader')),
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS user_scopes (
 grant_id INTEGER PRIMARY KEY AUTOINCREMENT,
 username TEXT NOT NULL, provider TEXT NOT NULL, profile TEXT NOT NULL,
 UNIQUE(username, provider, profile), FOREIGN KEY(username) REFERENCES users(username) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS consumer_state (
 username TEXT NOT NULL, provider TEXT NOT NULL, profile TEXT NOT NULL,
 last_seen TEXT NOT NULL, revision_seen INTEGER NOT NULL,
 PRIMARY KEY(username, provider, profile), FOREIGN KEY(username) REFERENCES users(username) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS consumer_reports (
 provider TEXT NOT NULL, profile TEXT NOT NULL, revision INTEGER NOT NULL, username TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('ok','authentication_required','access_denied','rate_limited','unknown_failure')),
 reported_at TEXT NOT NULL, grant_id INTEGER NOT NULL,
 PRIMARY KEY(provider, profile, revision, username),
 FOREIGN KEY(provider, profile, revision) REFERENCES revisions(provider, profile, revision) ON DELETE CASCADE,
 FOREIGN KEY(username) REFERENCES users(username) ON DELETE CASCADE);
CREATE INDEX IF NOT EXISTS consumer_reports_current ON consumer_reports(provider, profile, revision, kind);`)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS diagnostics_query ON diagnostics(provider, profile, installation_id, timestamp DESC, id DESC)`); err != nil {
		return err
	}
	return nil
}

func (s *Store) PutDiagnostics(ctx context.Context, provider, profile string, batch DiagnosticBatch) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, event := range batch.Events {
		plain, err := gzipJSON(event)
		if err != nil {
			return 0, err
		}
		nonce := make([]byte, s.aead.NonceSize())
		if _, err := cryptoRead(nonce); err != nil {
			return 0, err
		}
		ciphertext := s.aead.Seal(nil, nonce, plain, []byte("diagnostic:"+provider+":"+profile+":"+batch.InstallationID))
		if _, err := tx.ExecContext(ctx, `INSERT INTO diagnostics(provider,profile,installation_id,timestamp,severity,event_type,nonce,ciphertext) VALUES(?,?,?,?,?,?,?,?)`, provider, profile, batch.InstallationID, event.Timestamp.UTC().Format(time.RFC3339Nano), event.Severity, event.Type, nonce, ciphertext); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM diagnostics WHERE id IN (SELECT id FROM diagnostics WHERE timestamp < ? OR id NOT IN (SELECT id FROM diagnostics ORDER BY timestamp DESC, id DESC LIMIT 10000))`, time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(batch.Events), nil
}

func gzipJSON(value any) ([]byte, error) {
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func ungzipJSON(value []byte, target any) error {
	reader, err := gzip.NewReader(bytes.NewReader(value))
	if err != nil {
		return err
	}
	defer reader.Close()
	return json.NewDecoder(reader).Decode(target)
}

func (s *Store) Diagnostics(ctx context.Context, provider, profile, installation, severity, eventType string, from, to time.Time, beforeID int64, limit int) ([]DiagnosticRecord, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	query := `SELECT id,installation_id,timestamp,severity,event_type,nonce,ciphertext FROM diagnostics WHERE provider=? AND profile=?`
	args := []any{provider, profile}
	if installation != "" {
		query += ` AND installation_id=?`
		args = append(args, installation)
	}
	if severity != "" {
		query += ` AND severity=?`
		args = append(args, severity)
	}
	if eventType != "" {
		query += ` AND event_type=?`
		args = append(args, eventType)
	}
	if !from.IsZero() {
		query += ` AND timestamp>=?`
		args = append(args, from.UTC().Format(time.RFC3339Nano))
	}
	if !to.IsZero() {
		query += ` AND timestamp<=?`
		args = append(args, to.UTC().Format(time.RFC3339Nano))
	}
	if beforeID > 0 {
		query += ` AND id<?`
		args = append(args, beforeID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []DiagnosticRecord
	for rows.Next() {
		var record DiagnosticRecord
		var timestamp string
		var nonce, ciphertext []byte
		if err := rows.Scan(&record.ID, &record.InstallationID, &timestamp, &record.Event.Severity, &record.Event.Type, &nonce, &ciphertext); err != nil {
			return nil, err
		}
		record.Provider = provider
		record.Profile = profile
		plain, err := s.aead.Open(nil, nonce, ciphertext, []byte("diagnostic:"+provider+":"+profile+":"+record.InstallationID))
		if err != nil {
			return nil, fmt.Errorf("decrypt diagnostics: %w", err)
		}
		if err := ungzipJSON(plain, &record.Event); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *Store) Put(ctx context.Context, provider, profile, hash string, publicationReason PublicationReason, capturedAt time.Time, cookies []Cookie, authExpiresAt *time.Time, authExpirySource string) (Status, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Status{}, err
	}
	defer tx.Rollback()
	var current Status
	var currentCaptured, currentCreated string
	var currentAuthExpiresAt sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT r.provider,r.profile,r.revision,r.sha256,r.cookie_count,r.captured_at,r.created_at,r.publication_reason,r.auth_expires_at,r.auth_expiry_source FROM revisions r JOIN current_revisions c ON c.provider=r.provider AND c.profile=r.profile AND c.revision=r.revision WHERE r.provider=? AND r.profile=?`, provider, profile).
		Scan(&current.Provider, &current.Profile, &current.Revision, &current.SHA256, &current.CookieCount, &currentCaptured, &currentCreated, &current.PublicationReason, &currentAuthExpiresAt, &current.AuthExpirySource)
	if err == nil {
		current.CapturedAt, err = time.Parse(time.RFC3339, currentCaptured)
		if err == nil {
			current.CreatedAt, err = time.Parse(time.RFC3339, currentCreated)
		}
		if err == nil && currentAuthExpiresAt.Valid {
			parsed, parseErr := time.Parse(time.RFC3339, currentAuthExpiresAt.String)
			if parseErr != nil {
				err = parseErr
			} else {
				current.AuthExpiresAt = &parsed
			}
		}
	}
	if err == nil && current.SHA256 == hash && publicationReason != PublicationRecovery {
		current.Changed = false
		if err := tx.Commit(); err != nil {
			return Status{}, err
		}
		if err := s.enrichStatus(ctx, &current); err != nil {
			return Status{}, err
		}
		return current, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Status{}, err
	}
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0)+1 FROM revisions WHERE provider=? AND profile=?`, provider, profile).Scan(&revision); err != nil {
		return Status{}, err
	}
	plaint, err := json.Marshal(cookies)
	if err != nil {
		return Status{}, err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := cryptoRead(nonce); err != nil {
		return Status{}, err
	}
	aad := []byte(fmt.Sprintf("%s:%s:%d", provider, profile, revision))
	ciphertext := s.aead.Seal(nil, nonce, plaint, aad)
	now := time.Now().UTC().Truncate(time.Second)
	capturedAt = capturedAt.UTC().Truncate(time.Second)
	var authExpiresValue any
	if authExpiresAt != nil {
		formatted := authExpiresAt.UTC().Truncate(time.Second).Format(time.RFC3339)
		authExpiresValue = formatted
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO revisions(provider,profile,revision,created_at,captured_at,sha256,cookie_count,publication_reason,auth_expires_at,auth_expiry_source,nonce,ciphertext) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, provider, profile, revision, now.Format(time.RFC3339), capturedAt.Format(time.RFC3339), hash, len(cookies), publicationReason, authExpiresValue, authExpirySource, nonce, ciphertext); err != nil {
		return Status{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO current_revisions(provider,profile,revision) VALUES(?,?,?) ON CONFLICT(provider,profile) DO UPDATE SET revision=excluded.revision`, provider, profile, revision); err != nil {
		return Status{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM revisions WHERE provider=? AND profile=? AND revision NOT IN (SELECT revision FROM revisions WHERE provider=? AND profile=? ORDER BY revision DESC LIMIT 5)`, provider, profile, provider, profile); err != nil {
		return Status{}, err
	}
	if err := tx.Commit(); err != nil {
		return Status{}, err
	}
	return Status{Provider: provider, Profile: profile, Revision: revision, SHA256: hash, CookieCount: len(cookies), CapturedAt: capturedAt, CreatedAt: now, Changed: true, PublicationReason: publicationReason, AuthHealth: "healthy", AuthExpiresAt: authExpiresAt, AuthExpirySource: authExpirySource}, nil
}

func (s *Store) Current(ctx context.Context, provider, profile string) (storedSnapshot, error) {
	var out storedSnapshot
	var nonce, ciphertext []byte
	var capturedAt, createdAt string
	var authExpiresAt sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT r.provider,r.profile,r.revision,r.sha256,r.cookie_count,r.captured_at,r.created_at,r.publication_reason,r.auth_expires_at,r.auth_expiry_source,r.nonce,r.ciphertext FROM revisions r JOIN current_revisions c ON c.provider=r.provider AND c.profile=r.profile AND c.revision=r.revision WHERE r.provider=? AND r.profile=?`, provider, profile).
		Scan(&out.Status.Provider, &out.Status.Profile, &out.Status.Revision, &out.Status.SHA256, &out.Status.CookieCount, &capturedAt, &createdAt, &out.Status.PublicationReason, &authExpiresAt, &out.Status.AuthExpirySource, &nonce, &ciphertext)
	if err != nil {
		return out, err
	}
	out.Status.CapturedAt, err = time.Parse(time.RFC3339, capturedAt)
	if err != nil {
		return out, err
	}
	out.Status.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return out, err
	}
	if authExpiresAt.Valid {
		parsed, parseErr := time.Parse(time.RFC3339, authExpiresAt.String)
		if parseErr != nil {
			return out, parseErr
		}
		out.Status.AuthExpiresAt = &parsed
	}
	aad := []byte(fmt.Sprintf("%s:%s:%d", provider, profile, out.Status.Revision))
	plain, err := s.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return out, fmt.Errorf("decrypt snapshot: %w", err)
	}
	if err := json.Unmarshal(plain, &out.Cookies); err != nil {
		return out, err
	}
	if err := s.enrichStatus(ctx, &out.Status); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) Rollback(ctx context.Context, provider, profile string, revision int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE current_revisions SET revision=? WHERE provider=? AND profile=? AND EXISTS(SELECT 1 FROM revisions WHERE provider=? AND profile=? AND revision=?)`, revision, provider, profile, provider, profile, revision)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("revision not found")
	}
	return nil
}
