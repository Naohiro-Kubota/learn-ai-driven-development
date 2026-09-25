CREATE TABLE oidc_identities (id text PRIMARY KEY, issuer text NOT NULL, subject text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE (issuer, subject));
CREATE TABLE member_oidc_identities (identity_id text NOT NULL REFERENCES oidc_identities(id), member_id text NOT NULL REFERENCES members(id), PRIMARY KEY (identity_id, member_id));
CREATE TABLE organization_selection_transactions (id text PRIMARY KEY, cookie_hash bytea NOT NULL UNIQUE, csrf_token_hash bytea NOT NULL, identity_id text NOT NULL REFERENCES oidc_identities(id), expires_at timestamptz NOT NULL, consumed_at timestamptz);
CREATE TABLE organization_selection_transaction_members (transaction_id text NOT NULL REFERENCES organization_selection_transactions(id), member_id text NOT NULL REFERENCES members(id), PRIMARY KEY (transaction_id, member_id));
ALTER TABLE oidc_auth_transactions ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
