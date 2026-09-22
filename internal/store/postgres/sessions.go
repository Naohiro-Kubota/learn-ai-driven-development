package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
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

func (r *Repository) IdentityID(ctx context.Context, issuer, subject string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM oidc_identities WHERE issuer = $1 AND subject = $2`, issuer, subject).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", auth.ErrNotFound
	}
	return id, err
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
	var storedState []byte
	var consumed sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT id, state_hash, nonce, encrypted_verifier, issuer, client_id, redirect_uri, expires_at, consumed_at FROM oidc_auth_transactions WHERE cookie_hash=$1 FOR UPDATE`, ch[:]).Scan(&result.ID, &storedState, &result.Nonce, &result.EncryptedVerifier, &result.Issuer, &result.ClientID, &result.RedirectURI, &result.ExpiresAt, &consumed)
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
	if !equalBytes(storedState, sh[:]) {
		return auth.AuthTransaction{}, auth.ErrForbidden
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

func (r *Repository) Authenticate(ctx context.Context, cookie string, now time.Time) (auth.AuthenticatedSession, error) {
	hash := sha256.Sum256([]byte(cookie))
	var session auth.AuthenticatedSession
	err := r.db.QueryRowContext(ctx, `SELECT id, member_id, csrf_token_hash FROM app_sessions WHERE cookie_hash = $1 AND revoked_at IS NULL AND idle_expires_at > $2 AND absolute_expires_at > $2`, hash[:], now).Scan(&session.ID, &session.Principal.MemberID, &session.CSRFTokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.AuthenticatedSession{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.AuthenticatedSession{}, err
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE app_sessions SET last_used_at = $1 WHERE id = $2`, now, session.ID); err != nil {
		return auth.AuthenticatedSession{}, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT role FROM member_roles WHERE member_id = $1 ORDER BY role`, session.Principal.MemberID)
	if err != nil {
		return auth.AuthenticatedSession{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var role domain.Role
		if err := rows.Scan(&role); err != nil {
			return auth.AuthenticatedSession{}, err
		}
		session.Principal.Roles = append(session.Principal.Roles, role)
	}
	if err := rows.Err(); err != nil {
		return auth.AuthenticatedSession{}, err
	}
	return session, nil
}

func (r *Repository) IssueCSRFToken(ctx context.Context, sessionID string, now time.Time) (string, error) {
	token, err := auth.NewCSRFToken()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(token))
	result, err := r.db.ExecContext(ctx, `UPDATE app_sessions SET csrf_token_hash = $1 WHERE id = $2 AND revoked_at IS NULL AND idle_expires_at > $3 AND absolute_expires_at > $3`, hash[:], sessionID, now)
	if err != nil {
		return "", err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if updated != 1 {
		return "", auth.ErrNotFound
	}
	return token, nil
}

func (r *Repository) RevokeSession(ctx context.Context, cookie string, now time.Time) error {
	hash := sha256.Sum256([]byte(cookie))
	_, err := r.db.ExecContext(ctx, `UPDATE app_sessions SET revoked_at = $1 WHERE cookie_hash = $2 AND revoked_at IS NULL`, now, hash[:])
	return err
}

func (r *Repository) Revoke(ctx context.Context, cookie string, now time.Time) error {
	return r.RevokeSession(ctx, cookie, now)
}

func (r *Repository) ReadAndIssueCSRFToken(ctx context.Context, cookie string, now time.Time) (auth.OrganizationSelection, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return auth.OrganizationSelection{}, err
	}
	defer tx.Rollback()
	hash := sha256.Sum256([]byte(cookie))
	var id string
	var expiresAt time.Time
	var consumedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT id, expires_at, consumed_at FROM organization_selection_transactions WHERE cookie_hash = $1 FOR UPDATE`, hash[:]).Scan(&id, &expiresAt, &consumedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.OrganizationSelection{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.OrganizationSelection{}, err
	}
	if consumedAt.Valid {
		return auth.OrganizationSelection{}, auth.ErrConsumed
	}
	if !expiresAt.After(now) {
		return auth.OrganizationSelection{}, auth.ErrExpired
	}
	rows, err := tx.QueryContext(ctx, `SELECT m.id, o.id, o.name FROM organization_selection_transaction_members stm JOIN members m ON m.id = stm.member_id JOIN organizations o ON o.id = m.organization_id WHERE stm.transaction_id = $1 ORDER BY m.id`, id)
	if err != nil {
		return auth.OrganizationSelection{}, err
	}
	defer rows.Close()
	selection := auth.OrganizationSelection{}
	for rows.Next() {
		var candidate auth.OrganizationSelectionCandidate
		if err := rows.Scan(&candidate.MemberID, &candidate.OrganizationID, &candidate.OrganizationName); err != nil {
			return auth.OrganizationSelection{}, err
		}
		selection.Candidates = append(selection.Candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return auth.OrganizationSelection{}, err
	}
	token, err := auth.NewCSRFToken()
	if err != nil {
		return auth.OrganizationSelection{}, err
	}
	tokenHash := sha256.Sum256([]byte(token))
	if _, err := tx.ExecContext(ctx, `UPDATE organization_selection_transactions SET csrf_token_hash = $1 WHERE id = $2`, tokenHash[:], id); err != nil {
		return auth.OrganizationSelection{}, err
	}
	if err := tx.Commit(); err != nil {
		return auth.OrganizationSelection{}, err
	}
	selection.CSRFToken = token
	return selection, nil
}

func (r *Repository) CompleteOrganizationSelection(ctx context.Context, input auth.CompleteOrganizationSelectionInput, session auth.SessionInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ch, csrf := sha256.Sum256([]byte(input.Cookie)), sha256.Sum256([]byte(input.CSRFToken))
	var id string
	var expires time.Time
	var consumed sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT id, expires_at, consumed_at FROM organization_selection_transactions WHERE cookie_hash = $1 FOR UPDATE`, ch[:]).Scan(&id, &expires, &consumed)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.ErrNotFound
	}
	if err != nil {
		return err
	}
	if consumed.Valid {
		return auth.ErrConsumed
	}
	if _, err := tx.ExecContext(ctx, `UPDATE organization_selection_transactions SET consumed_at = $1 WHERE id = $2`, input.Now, id); err != nil {
		return err
	}
	if !expires.After(input.Now) {
		if err := tx.Commit(); err != nil {
			return err
		}
		return auth.ErrExpired
	}
	var storedCSRF []byte
	if err := tx.QueryRowContext(ctx, `SELECT csrf_token_hash FROM organization_selection_transactions WHERE id = $1`, id).Scan(&storedCSRF); err != nil {
		return err
	}
	if !equalBytes(storedCSRF, csrf[:]) {
		if err := tx.Commit(); err != nil {
			return err
		}
		return auth.ErrCSRFValidation
	}
	var candidate bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM organization_selection_transaction_members WHERE transaction_id = $1 AND member_id = $2)`, id, input.MemberID).Scan(&candidate); err != nil {
		return err
	}
	if !candidate {
		if err := tx.Commit(); err != nil {
			return err
		}
		return auth.ErrForbidden
	}
	if session.MemberID != input.MemberID {
		if err := tx.Commit(); err != nil {
			return err
		}
		return auth.ErrForbidden
	}
	cookieHash, csrfHash := sha256.Sum256([]byte(session.Cookie)), sha256.Sum256([]byte(session.CSRFToken))
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_sessions (id,cookie_hash,member_id,csrf_token_hash,created_at,last_used_at,idle_expires_at,absolute_expires_at) VALUES ($1,$2,$3,$4,$5,$5,$6,$7)`, session.ID, cookieHash[:], session.MemberID, csrfHash[:], session.CreatedAt, session.IdleExpiresAt, session.AbsoluteExpiresAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *Repository) ConsumeOrganizationSelection(ctx context.Context, input auth.ConsumeOrganizationSelectionInput) (auth.Session, error) {
	if err := r.CompleteOrganizationSelection(ctx, auth.CompleteOrganizationSelectionInput{Cookie: input.Cookie, CSRFToken: input.CSRFToken, MemberID: input.MemberID, Now: input.Now}, input.Session); err != nil {
		return auth.Session{}, err
	}
	return auth.Session{ID: input.Session.ID, MemberID: input.Session.MemberID, CreatedAt: input.Session.CreatedAt, IdleExpiresAt: input.Session.IdleExpiresAt, AbsoluteExpiresAt: input.Session.AbsoluteExpiresAt}, nil
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
