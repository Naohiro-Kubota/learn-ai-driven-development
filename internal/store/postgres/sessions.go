package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
)

func (r *Repository) MembersForIdentity(ctx context.Context, issuer, subject string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT m.member_id FROM member_oidc_identities m JOIN oidc_identities i ON i.id = m.identity_id WHERE i.issuer = $1 AND i.subject = $2 ORDER BY m.member_id`, issuer, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) CreateOrganizationSelection(ctx context.Context, input auth.OrganizationSelectionInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ch, csrf := sha256.Sum256([]byte(input.Cookie)), sha256.Sum256([]byte(input.CSRFToken))
	if _, err = tx.ExecContext(ctx, `INSERT INTO organization_selection_transactions (id, cookie_hash, csrf_token_hash, identity_id, expires_at) VALUES ($1,$2,$3,$4,$5)`, input.ID, ch[:], csrf[:], input.IdentityID, input.ExpiresAt); err != nil {
		return err
	}
	for _, memberID := range input.MemberIDs {
		if _, err = tx.ExecContext(ctx, `INSERT INTO organization_selection_transaction_members (transaction_id, member_id) VALUES ($1,$2)`, input.ID, memberID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) CreateSession(ctx context.Context, input auth.SessionInput) error {
	cookieHash, csrfHash := sha256.Sum256([]byte(input.Cookie)), sha256.Sum256([]byte(input.CSRFToken))
	_, err := r.db.ExecContext(ctx, `INSERT INTO app_sessions (id,cookie_hash,member_id,csrf_token_hash,created_at,last_used_at,idle_expires_at,absolute_expires_at) VALUES ($1,$2,$3,$4,$5,$5,$6,$7)`, input.ID, cookieHash[:], input.MemberID, csrfHash[:], input.CreatedAt, input.IdleExpiresAt, input.AbsoluteExpiresAt)
	return err
}

func (r *Repository) CreateAuthTransaction(ctx context.Context, input auth.AuthTransaction) error {
	cookieHash, stateHash := sha256.Sum256([]byte(input.Cookie)), sha256.Sum256([]byte(input.State))
	_, err := r.db.ExecContext(ctx, `INSERT INTO oidc_auth_transactions (id,cookie_hash,state_hash,nonce,encrypted_verifier,issuer,client_id,redirect_uri,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, input.ID, cookieHash[:], stateHash[:], input.Nonce, input.EncryptedVerifier, input.Issuer, input.ClientID, input.RedirectURI, input.ExpiresAt)
	return err
}

func (r *Repository) ConsumeAuthTransaction(ctx context.Context, cookie, state string, now time.Time) (auth.AuthTransaction, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return auth.AuthTransaction{}, err
	}
	defer tx.Rollback()
	ch, sh := sha256.Sum256([]byte(cookie)), sha256.Sum256([]byte(state))
	var result auth.AuthTransaction
	var consumed sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT id, nonce, encrypted_verifier, issuer, client_id, redirect_uri, expires_at, consumed_at FROM oidc_auth_transactions WHERE cookie_hash=$1 AND state_hash=$2 FOR UPDATE`, ch[:], sh[:]).Scan(&result.ID, &result.Nonce, &result.EncryptedVerifier, &result.Issuer, &result.ClientID, &result.RedirectURI, &result.ExpiresAt, &consumed)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.AuthTransaction{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.AuthTransaction{}, err
	}
	if consumed.Valid {
		return auth.AuthTransaction{}, auth.ErrConsumed
	}
	if _, err := tx.ExecContext(ctx, `UPDATE oidc_auth_transactions SET consumed_at=$1 WHERE id=$2`, now, result.ID); err != nil {
		return auth.AuthTransaction{}, err
	}
	if err := tx.Commit(); err != nil {
		return auth.AuthTransaction{}, err
	}
	if !result.ExpiresAt.After(now) {
		return auth.AuthTransaction{}, auth.ErrExpired
	}
	return result, nil
}

func (r *Repository) AuthenticateSession(ctx context.Context, cookie string, now time.Time) (auth.Session, error) {
	hash := sha256.Sum256([]byte(cookie))
	var session auth.Session
	err := r.db.QueryRowContext(ctx, `SELECT id, member_id, created_at, idle_expires_at, absolute_expires_at FROM app_sessions WHERE cookie_hash = $1 AND revoked_at IS NULL AND idle_expires_at > $2 AND absolute_expires_at > $2`, hash[:], now).Scan(&session.ID, &session.MemberID, &session.CreatedAt, &session.IdleExpiresAt, &session.AbsoluteExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Session{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.Session{}, err
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE app_sessions SET last_used_at = $1 WHERE id = $2`, now, session.ID); err != nil {
		return auth.Session{}, err
	}
	return session, nil
}

func (r *Repository) RevokeSession(ctx context.Context, cookie string, now time.Time) error {
	hash := sha256.Sum256([]byte(cookie))
	_, err := r.db.ExecContext(ctx, `UPDATE app_sessions SET revoked_at = $1 WHERE cookie_hash = $2 AND revoked_at IS NULL`, now, hash[:])
	return err
}

func (r *Repository) ConsumeOrganizationSelection(ctx context.Context, input auth.ConsumeOrganizationSelectionInput) (auth.Session, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return auth.Session{}, err
	}
	defer tx.Rollback()
	ch, csrf := sha256.Sum256([]byte(input.Cookie)), sha256.Sum256([]byte(input.CSRFToken))
	var id string
	var expires time.Time
	var consumed sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT id, expires_at, consumed_at FROM organization_selection_transactions WHERE cookie_hash = $1 FOR UPDATE`, ch[:]).Scan(&id, &expires, &consumed)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Session{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.Session{}, err
	}
	_, _ = tx.ExecContext(ctx, `UPDATE organization_selection_transactions SET consumed_at = $1 WHERE id = $2 AND consumed_at IS NULL`, input.Now, id)
	if consumed.Valid {
		return auth.Session{}, auth.ErrConsumed
	}
	if !expires.After(input.Now) {
		if err := tx.Commit(); err != nil {
			return auth.Session{}, err
		}
		return auth.Session{}, auth.ErrExpired
	}
	var storedCSRF []byte
	if err := tx.QueryRowContext(ctx, `SELECT csrf_token_hash FROM organization_selection_transactions WHERE id = $1`, id).Scan(&storedCSRF); err != nil {
		return auth.Session{}, err
	}
	if !equalBytes(storedCSRF, csrf[:]) {
		if err := tx.Commit(); err != nil {
			return auth.Session{}, err
		}
		return auth.Session{}, auth.ErrForbidden
	}
	var candidate bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM organization_selection_transaction_members WHERE transaction_id = $1 AND member_id = $2)`, id, input.MemberID).Scan(&candidate); err != nil {
		return auth.Session{}, err
	}
	if !candidate {
		if err := tx.Commit(); err != nil {
			return auth.Session{}, err
		}
		return auth.Session{}, auth.ErrForbidden
	}
	s := input.Session
	cookieHash, csrfHash := sha256.Sum256([]byte(s.Cookie)), sha256.Sum256([]byte(s.CSRFToken))
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_sessions (id,cookie_hash,member_id,csrf_token_hash,created_at,last_used_at,idle_expires_at,absolute_expires_at) VALUES ($1,$2,$3,$4,$5,$5,$6,$7)`, s.ID, cookieHash[:], s.MemberID, csrfHash[:], s.CreatedAt, s.IdleExpiresAt, s.AbsoluteExpiresAt); err != nil {
		return auth.Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return auth.Session{}, err
	}
	return auth.Session{ID: s.ID, MemberID: s.MemberID, CreatedAt: s.CreatedAt, IdleExpiresAt: s.IdleExpiresAt, AbsoluteExpiresAt: s.AbsoluteExpiresAt}, nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var x byte
	for i := range a {
		x |= a[i] ^ b[i]
	}
	return x == 0
}
