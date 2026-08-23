package broker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/segfault-dev/media-cookie-broker/internal/providers"
)

func validateScope(scope Scope) error {
	if !providers.ValidID(scope.Provider) {
		return fmt.Errorf("unknown provider %q", scope.Provider)
	}
	if !ValidProfileID(scope.Profile) {
		return fmt.Errorf("invalid profile %q", scope.Profile)
	}
	return nil
}

func (s *Store) CreateUser(ctx context.Context, username, password, role string, scopes []Scope) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.createUserWithHash(ctx, username, hash, role, scopes)
}

func (s *Store) createUserWithHash(ctx context.Context, username, passwordHash, role string, scopes []Scope) error {
	if !ValidUsername(username) {
		return errors.New("invalid username")
	}
	if !ValidRole(role) {
		return errors.New("invalid role")
	}
	if _, _, _, _, _, err := decodeArgon2(passwordHash); err != nil {
		return fmt.Errorf("invalid password hash: %w", err)
	}
	for _, scope := range scopes {
		if err := validateScope(scope); err != nil {
			return err
		}
	}
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO users(username,password_hash,role,created_at,updated_at) VALUES(?,?,?,?,?)`, username, passwordHash, role, now, now); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	for _, scope := range scopes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_scopes(username,provider,profile) VALUES(?,?,?)`, username, scope.Provider, scope.Profile); err != nil {
			return fmt.Errorf("grant scope: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) ListUsers(ctx context.Context) ([]UserRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT username,role,created_at,updated_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []UserRecord
	for rows.Next() {
		var user UserRecord
		var createdAt, updatedAt string
		if err := rows.Scan(&user.Username, &user.Role, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		user.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		user.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range users {
		users[i].Scopes, err = s.scopesForUser(ctx, users[i].Username)
		if err != nil {
			return nil, err
		}
	}
	return users, nil
}

func (s *Store) DeleteUser(ctx context.Context, username string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE username=?`, username)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("user not found")
	}
	return nil
}

func (s *Store) ChangePassword(ctx context.Context, username, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash=?,updated_at=? WHERE username=?`, hash, time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), username)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("user not found")
	}
	return nil
}

func (s *Store) Grant(ctx context.Context, username, provider, profile string) error {
	if err := validateScope(Scope{Provider: provider, Profile: profile}); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO user_scopes(username,provider,profile) SELECT username,?,? FROM users WHERE username=? ON CONFLICT DO NOTHING`, provider, profile, username)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username=?`, username).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return errors.New("user not found")
		}
		return errors.New("scope already granted")
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET updated_at=? WHERE username=?`, time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), username)
	return err
}

func (s *Store) Revoke(ctx context.Context, username, provider, profile string) error {
	if err := validateScope(Scope{Provider: provider, Profile: profile}); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM user_scopes WHERE username=? AND provider=? AND profile=?`, username, provider, profile)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("scope not found")
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET updated_at=? WHERE username=?`, time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), username)
	return err
}

func (s *Store) userPasswordHash(ctx context.Context, username, role, provider, profile string) (string, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT u.password_hash FROM users u JOIN user_scopes s ON s.username=u.username WHERE u.username=? AND u.role=? AND s.provider=? AND s.profile=?`, username, role, provider, profile).Scan(&hash)
	return hash, err
}

func (s *Store) userRolePasswordHash(ctx context.Context, username, role string) (string, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE username=? AND role=?`, username, role).Scan(&hash)
	return hash, err
}

func (s *Store) scopesForUser(ctx context.Context, username string) ([]Scope, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT provider,profile FROM user_scopes WHERE username=? ORDER BY provider,profile`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scopes []Scope
	for rows.Next() {
		var scope Scope
		if err := rows.Scan(&scope.Provider, &scope.Profile); err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, rows.Err()
}

func (s *Store) ScopesForUser(ctx context.Context, username string) ([]Scope, error) {
	return s.scopesForUser(ctx, username)
}
