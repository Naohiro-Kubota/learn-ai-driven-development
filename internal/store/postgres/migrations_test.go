package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestMigrationsRequireTestDatabaseURL(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Fatal("TEST_DATABASE_URL is required")
	}
}

func TestMigrationsCreateWorkflowAndAuthTables(t *testing.T) {
	db, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := migrate.New("file://../../../migrations", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"requests", "approvals", "audit_events", "app_sessions", "oidc_auth_transactions"} {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("table %s does not exist", table)
		}
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-default', 'Default organization'), ('org-other', 'Other organization')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO members (id, organization_id, oidc_subject) VALUES ('approver-default', 'org-default', 'approver-default'), ('approver-other', 'org-other', 'approver-other')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE organizations SET default_approver_member_id = 'approver-default' WHERE id = 'org-default'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE organizations SET default_approver_member_id = 'approver-other' WHERE id = 'org-default'`); err == nil {
		t.Fatal("cross-organization default approver was accepted")
	}
	if err := m.Down(); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('requests', 'approvals', 'audit_events', 'app_sessions', 'oidc_auth_transactions')`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("migration tables remain after down: %d", remaining)
	}
}
