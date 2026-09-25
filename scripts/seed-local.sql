BEGIN;
INSERT INTO organizations (id, name) VALUES ('local-organization', 'Local organization')
  ON CONFLICT (id) DO NOTHING;
INSERT INTO members (id, organization_id, oidc_subject) VALUES
  ('local-requester', 'local-organization', :'requester_subject'),
  ('local-approver', 'local-organization', :'approver_subject'),
  ('local-admin', 'local-organization', :'admin_subject')
  ON CONFLICT (id) DO UPDATE SET oidc_subject = EXCLUDED.oidc_subject;
INSERT INTO member_roles (member_id, role) VALUES
  ('local-requester', 'requester'), ('local-approver', 'approver'), ('local-admin', 'admin')
  ON CONFLICT DO NOTHING;
INSERT INTO oidc_identities (id, issuer, subject) VALUES
  ('local-requester-identity', 'http://127.0.0.1:8081/realms/approval-flow-dev', :'requester_subject'),
  ('local-approver-identity', 'http://127.0.0.1:8081/realms/approval-flow-dev', :'approver_subject'),
  ('local-admin-identity', 'http://127.0.0.1:8081/realms/approval-flow-dev', :'admin_subject')
  ON CONFLICT (id) DO UPDATE SET subject = EXCLUDED.subject;
INSERT INTO member_oidc_identities (identity_id, member_id) VALUES
  ('local-requester-identity', 'local-requester'),
  ('local-approver-identity', 'local-approver'),
  ('local-admin-identity', 'local-admin')
  ON CONFLICT DO NOTHING;
UPDATE organizations SET default_approver_member_id = 'local-approver' WHERE id = 'local-organization';
COMMIT;
