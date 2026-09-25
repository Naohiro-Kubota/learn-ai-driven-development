package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type migrationRunner interface {
	Up() error
	Down() error
	Version() (uint, bool, error)
	Close() (sourceErr error, databaseErr error)
}

var newMigration = func(sourceURL, databaseURL string) (migrationRunner, error) {
	return migrate.New(sourceURL, databaseURL)
}

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(args []string, lookup func(string) string, output io.Writer) error {
	flags := flag.NewFlagSet("migrate-local", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	databaseURL := flags.String("database-url", lookup("DATABASE_URL"), "database URL (defaults to DATABASE_URL)")

	command := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	if err := flags.Parse(args); err != nil {
		return errors.New("invalid migration options")
	}
	if flags.NArg() > 0 {
		if command != "" {
			return errors.New("only one migration command is allowed")
		}
		command = flags.Arg(0)
	}
	if command == "" {
		command = "up"
	}
	if strings.TrimSpace(*databaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	if command != "up" && command != "down" && command != "version" {
		return errors.New("unknown migration command")
	}
	if err := validateLocalDatabaseURL(*databaseURL); err != nil {
		return err
	}

	migration, err := newMigration("file://migrations", *databaseURL)
	if err != nil {
		return errors.New("unable to initialize migrations")
	}
	defer migration.Close()

	switch command {
	case "up":
		err = migration.Up()
	case "down":
		err = migration.Down()
	case "version":
		var version uint
		var dirty bool
		version, dirty, err = migration.Version()
		if err == nil {
			_, _ = fmt.Fprintf(output, "version=%d dirty=%t\n", version, dirty)
		}
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return errors.New("migration command failed")
	}
	if command != "version" {
		_, _ = fmt.Fprintf(output, "migration %s complete\n", command)
	}
	return nil
}

func validateLocalDatabaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" || parsed.Host == "" || parsed.Opaque != "" || parsed.Fragment != "" {
		return errors.New("DATABASE_URL must be a local PostgreSQL URL")
	}
	host := parsed.Hostname()
	if host == "" || host != "localhost" && !net.ParseIP(host).IsLoopback() {
		return errors.New("DATABASE_URL must target a loopback host")
	}
	if parsed.Port() == "" && strings.Contains(parsed.Host, ":") && net.ParseIP(host) == nil {
		return errors.New("DATABASE_URL must use a valid host")
	}
	if parsed.RawQuery != "" {
		query, err := url.ParseQuery(parsed.RawQuery)
		if err != nil {
			return errors.New("DATABASE_URL contains invalid query parameters")
		}
		for key := range query {
			if key != "sslmode" {
				return errors.New("DATABASE_URL contains unsupported query parameters")
			}
		}
	}
	return nil
}
