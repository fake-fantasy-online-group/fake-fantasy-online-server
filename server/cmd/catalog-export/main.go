// catalog-export creates local public data artifacts. It does not start any
// server, initialize any database, upload data, or deploy a Cloudflare project.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/publiccatalog"
	"github.com/jackc/pgx/v5"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "catalog-export:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("catalog-export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("out", "", "required local output directory (keep generated data outside Git)")
	sourceRevision := flags.String("source-revision", "", "optional public release/source label; never put secrets here")
	maxRows := flags.Int("max-rows", publiccatalog.DefaultMaxRows, "abort rather than truncate a larger catalog")
	timeout := flags.Duration("timeout", 2*time.Minute, "overall read-only export timeout")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: catalog-export --out DIRECTORY [--source-revision LABEL]")
		fmt.Fprintln(stderr, "Set CATALOG_EXPORT_DATABASE_URL to an explicitly approved read-only source database.")
		fmt.Fprintln(stderr, "DATABASE_URL and game server configuration are never used. No upload or deployment occurs.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || *out == "" {
		return errors.New("--out is required and positional arguments are not accepted")
	}
	if *timeout <= 0 || *timeout > time.Hour || *maxRows < 1 || *maxRows > 1000000 {
		return errors.New("timeout must be positive and at most 1h; max rows must be 1..1000000")
	}
	dsn := getenv("CATALOG_EXPORT_DATABASE_URL")
	if dsn == "" {
		return errors.New("CATALOG_EXPORT_DATABASE_URL is required; select an approved read-only source explicitly")
	}
	location, err := url.Parse(dsn)
	if err != nil || (location.Scheme != "postgres" && location.Scheme != "postgresql") ||
		(location.Hostname() == "" && location.Query().Get("host") == "") || location.Path == "" || location.Path == "/" {
		return errors.New("CATALOG_EXPORT_DATABASE_URL must be a PostgreSQL URL with explicit host and database")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		// Parse errors can contain connection strings. Do not print credentials.
		return errors.New("invalid CATALOG_EXPORT_DATABASE_URL connection settings")
	}
	config.RuntimeParams["default_transaction_read_only"] = "on"
	config.RuntimeParams["search_path"] = "pg_catalog"
	config.RuntimeParams["application_name"] = "public-catalog-export"
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return errors.New("could not connect to the selected catalog source; check connectivity and read-only credentials")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Close(cleanup)
	}()
	artifact, err := publiccatalog.Export(ctx, conn, publiccatalog.Options{MaxRows: *maxRows, SourceRevision: *sourceRevision})
	if err != nil {
		return err
	}
	if err := publiccatalog.Write(*out, artifact); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(artifact.Manifest)
}
