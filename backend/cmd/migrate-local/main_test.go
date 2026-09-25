package main

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeMigration struct {
	command string
}

func (f *fakeMigration) Up() error {
	f.command = "up"
	return nil
}

func (f *fakeMigration) Down() error {
	f.command = "down"
	return nil
}

func (f *fakeMigration) Version() (uint, bool, error) {
	f.command = "version"
	return 4, false, nil
}

func (f *fakeMigration) Close() (error, error) { return nil, nil }

func TestRunDefaultsToUpAndReadsDatabaseURLFromEnvironment(t *testing.T) {
	fake := &fakeMigration{}
	var gotSource, gotDatabase string
	original := newMigration
	newMigration = func(source, database string) (migrationRunner, error) {
		gotSource, gotDatabase = source, database
		return fake, nil
	}
	t.Cleanup(func() { newMigration = original })

	var output strings.Builder
	err := run(nil, func(name string) string {
		if name == "DATABASE_URL" {
			return "postgres://local:secret@127.0.0.1:5432/approval"
		}
		return ""
	}, &output)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if fake.command != "up" {
		t.Fatalf("migration command = %q, want up", fake.command)
	}
	if gotSource != "file://migrations" {
		t.Fatalf("source = %q, want file://migrations", gotSource)
	}
	if gotDatabase != "postgres://local:secret@127.0.0.1:5432/approval" {
		t.Fatalf("database URL was not passed to migrate")
	}
	if strings.Contains(output.String(), "secret") {
		t.Fatalf("output contains database credentials: %q", output.String())
	}
}

func TestRunRejectsEmptyDatabaseURLAndUnknownCommand(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		env  string
	}{
		{name: "empty URL", env: "   "},
		{name: "unknown command", args: []string{"reset"}, env: "postgres://local/db"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := run(test.args, func(name string) string {
				if name == "DATABASE_URL" {
					return test.env
				}
				return ""
			}, io.Discard)
			if err == nil {
				t.Fatal("run() error = nil, want error")
			}
		})
	}
}

func TestRunRejectsNonLoopbackAndMalformedDatabaseURLs(t *testing.T) {
	for _, databaseURL := range []string{
		"postgres://db.example.test/approval",
		"postgres://192.0.2.1/approval",
		"mysql://127.0.0.1/approval",
		"postgres://[::1/approval",
		"postgres:///approval",
	} {
		t.Run(databaseURL, func(t *testing.T) {
			called := false
			original := newMigration
			newMigration = func(string, string) (migrationRunner, error) {
				called = true
				return &fakeMigration{}, nil
			}
			t.Cleanup(func() { newMigration = original })

			err := run(nil, func(name string) string {
				if name == "DATABASE_URL" {
					return databaseURL
				}
				return ""
			}, io.Discard)
			if err == nil || called {
				t.Fatalf("run() error = %v, migration called = %v", err, called)
			}
			if strings.Contains(err.Error(), "approval") || strings.Contains(err.Error(), "db.example") {
				t.Fatalf("validation error exposes database URL: %v", err)
			}
		})
	}
}

func TestRunRejectsDatabaseURLQueryThatOverridesAuthority(t *testing.T) {
	called := false
	original := newMigration
	newMigration = func(string, string) (migrationRunner, error) {
		called = true
		return &fakeMigration{}, nil
	}
	t.Cleanup(func() { newMigration = original })

	err := run(nil, func(name string) string {
		if name == "DATABASE_URL" {
			return "postgres://127.0.0.1/approval?host=192.0.2.1"
		}
		return ""
	}, io.Discard)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if called {
		t.Fatal("migration factory was called for a rejected database URL")
	}
	if strings.Contains(err.Error(), "192.0.2.1") || strings.Contains(err.Error(), "approval") {
		t.Fatalf("validation error exposes database URL details: %v", err)
	}
}

func TestRunRejectsDatabaseURLQueryPortAuthorityOverride(t *testing.T) {
	called := false
	original := newMigration
	newMigration = func(string, string) (migrationRunner, error) {
		called = true
		return &fakeMigration{}, nil
	}
	t.Cleanup(func() { newMigration = original })

	err := run(nil, func(name string) string {
		if name == "DATABASE_URL" {
			return "postgres://127.0.0.1/approval?port=192.0.2.1"
		}
		return ""
	}, io.Discard)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if called {
		t.Fatal("migration factory was called for a rejected database URL")
	}
	if strings.Contains(err.Error(), "192.0.2.1") || strings.Contains(err.Error(), "approval") {
		t.Fatalf("validation error exposes database URL details: %v", err)
	}
}

func TestRunDatabaseFlagOverridesEnvironment(t *testing.T) {
	fake := &fakeMigration{}
	original := newMigration
	var gotDatabase string
	newMigration = func(_, database string) (migrationRunner, error) {
		gotDatabase = database
		return fake, nil
	}
	t.Cleanup(func() { newMigration = original })

	err := run([]string{"version", "-database-url", "postgres://127.0.0.1/flag"}, func(name string) string {
		if name == "DATABASE_URL" {
			return "postgres://127.0.0.1/env"
		}
		return ""
	}, io.Discard)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if fake.command != "version" || gotDatabase != "postgres://127.0.0.1/flag" {
		t.Fatalf("command/database = %q/%q", fake.command, gotDatabase)
	}
}

func TestRunSupportsExplicitDownAndVersionCommands(t *testing.T) {
	for _, command := range []string{"down", "version"} {
		t.Run(command, func(t *testing.T) {
			fake := &fakeMigration{}
			original := newMigration
			newMigration = func(string, string) (migrationRunner, error) { return fake, nil }
			t.Cleanup(func() { newMigration = original })

			if err := run([]string{command}, func(name string) string {
				if name == "DATABASE_URL" {
					return "postgres://localhost/approval"
				}
				return ""
			}, io.Discard); err != nil {
				t.Fatalf("run() error = %v", err)
			}
			if fake.command != command {
				t.Fatalf("migration command = %q, want %q", fake.command, command)
			}
		})
	}
}

func TestRunDoesNotExposeMigrationErrorDetails(t *testing.T) {
	original := newMigration
	newMigration = func(string, string) (migrationRunner, error) {
		return nil, errors.New("dial postgres://local:secret@127.0.0.1")
	}
	t.Cleanup(func() { newMigration = original })

	err := run(nil, func(name string) string {
		if name == "DATABASE_URL" {
			return "postgres://local:secret@127.0.0.1/db"
		}
		return ""
	}, io.Discard)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("error exposes credentials: %v", err)
	}
}

func TestRunAllowsOnlyComposePostgresInExplicitDevelopmentMode(t *testing.T) {
	for _, tc := range []struct {
		name, mode, env, host string
		allowed               bool
	}{
		{"compose", "true", "development", "postgres", true},
		{"production", "true", "production", "postgres", false},
		{"missing mode", "", "development", "postgres", false},
		{"other host", "true", "development", "remote", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			original := newMigration
			newMigration = func(_, _ string) (migrationRunner, error) { called = true; return &fakeMigration{}, nil }
			t.Cleanup(func() { newMigration = original })
			err := run(nil, func(key string) string {
				return map[string]string{"DATABASE_URL": "postgres://user:secret@" + tc.host + ":5432/db", "APP_LOCAL_COMPOSE": tc.mode, "APP_ENV": tc.env}[key]
			}, io.Discard)
			if tc.allowed != (err == nil && called) {
				t.Fatalf("err=%v called=%v", err, called)
			}
		})
	}
}
