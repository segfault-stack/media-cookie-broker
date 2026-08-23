package broker

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) RecordConsumerSeen(ctx context.Context, username, provider, profile string, revision int64) error {
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT INTO consumer_state(username,provider,profile,last_seen,revision_seen) VALUES(?,?,?,?,?)
ON CONFLICT(username,provider,profile) DO UPDATE SET last_seen=excluded.last_seen,revision_seen=excluded.revision_seen`, username, provider, profile, now, revision)
	return err
}

func (s *Store) ConsumerState(ctx context.Context, username, provider, profile string) (ConsumerState, error) {
	var state ConsumerState
	var lastSeen string
	err := s.db.QueryRowContext(ctx, `SELECT username,provider,profile,last_seen,revision_seen FROM consumer_state WHERE username=? AND provider=? AND profile=?`, username, provider, profile).
		Scan(&state.Username, &state.Provider, &state.Profile, &lastSeen, &state.RevisionSeen)
	if err != nil {
		return state, err
	}
	state.LastSeen, err = time.Parse(time.RFC3339, lastSeen)
	return state, err
}

func (s *Store) PutHealthReport(ctx context.Context, username, provider, profile string, report HealthReport) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM revisions WHERE provider=? AND profile=? AND revision=?`, provider, profile, report.Revision).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return errors.New("revision not found for provider/profile")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `INSERT INTO consumer_reports(provider,profile,revision,username,kind,reported_at,grant_id)
SELECT ?,?,?,u.username,?,?,s.grant_id
FROM users u JOIN user_scopes s ON s.username=u.username
WHERE u.username=? AND u.role='reader' AND s.provider=? AND s.profile=?
ON CONFLICT(provider,profile,revision,username) DO UPDATE SET kind=excluded.kind,reported_at=excluded.reported_at,grant_id=excluded.grant_id`,
		provider, profile, report.Revision, report.Kind, now, username, provider, profile)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("consumer is not authorized for provider/profile")
	}
	return nil
}

func (s *Store) enrichStatus(ctx context.Context, status *Status) error {
	status.AuthHealth = "healthy"
	status.CurrentReportCounts = map[string]int{}
	const activeReports = ` FROM consumer_reports cr
JOIN users u ON u.username=cr.username AND u.role='reader'
JOIN user_scopes s ON s.username=cr.username AND s.provider=cr.provider AND s.profile=cr.profile AND s.grant_id=cr.grant_id
WHERE cr.provider=? AND cr.profile=? AND cr.revision=?`
	rows, err := s.db.QueryContext(ctx, `SELECT cr.kind,COUNT(*)`+activeReports+` GROUP BY cr.kind`, status.Provider, status.Profile, status.Revision)
	if err != nil {
		return err
	}
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			rows.Close()
			return err
		}
		status.CurrentReportCounts[kind] = count
	}
	if err := rows.Close(); err != nil {
		return err
	}
	status.AuthRequiredCount = status.CurrentReportCounts["authentication_required"]
	if status.AuthRequiredCount > 0 {
		status.AuthHealth = "refresh_required"
	}
	var lastReport sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(cr.reported_at)`+activeReports, status.Provider, status.Profile, status.Revision).Scan(&lastReport); err != nil {
		return err
	}
	if lastReport.Valid {
		parsed, err := time.Parse(time.RFC3339, lastReport.String)
		if err != nil {
			return err
		}
		status.LastReportAt = &parsed
	}
	if len(status.CurrentReportCounts) == 0 {
		status.CurrentReportCounts = nil
	}
	return nil
}

func (s *Store) StatusesForScopes(ctx context.Context, scopes []Scope) ([]Status, error) {
	statuses := make([]Status, 0, len(scopes))
	for _, scope := range scopes {
		snapshot, err := s.Current(ctx, scope.Provider, scope.Profile)
		if errors.Is(err, sql.ErrNoRows) {
			statuses = append(statuses, Status{Provider: scope.Provider, Profile: scope.Profile, AuthHealth: "missing"})
			continue
		}
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, snapshot.Status)
	}
	return statuses, nil
}
