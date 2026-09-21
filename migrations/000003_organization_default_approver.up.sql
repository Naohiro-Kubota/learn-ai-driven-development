ALTER TABLE members ADD CONSTRAINT members_id_organization_id_key UNIQUE (id, organization_id);

ALTER TABLE organizations ADD COLUMN default_approver_member_id text;

ALTER TABLE organizations
  ADD CONSTRAINT organizations_default_approver_same_organization_fkey
  FOREIGN KEY (default_approver_member_id, id)
  REFERENCES members (id, organization_id);
