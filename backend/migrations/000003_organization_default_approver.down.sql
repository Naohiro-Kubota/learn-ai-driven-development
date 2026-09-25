ALTER TABLE organizations
  DROP CONSTRAINT organizations_default_approver_same_organization_fkey;

ALTER TABLE organizations DROP COLUMN default_approver_member_id;

ALTER TABLE members DROP CONSTRAINT members_id_organization_id_key;
