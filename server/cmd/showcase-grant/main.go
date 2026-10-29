// showcase-grant is deliberately offline-only, bounded to test001/42,183,206.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/showcase"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	apply := flag.Bool("apply", false, "Commit the verified 27-item grant (default: read-only dry-run)")
	maintenance := flag.Bool("maintenance-confirmed", false, "Confirm every game service stopped after normal save, and restarts disabled throughout this invocation")
	reportPath := flag.String("report", "", "New private JSON audit report path (must not exist)")
	dsnEnv := flag.String("dsn-env", "DATABASE_URL", "Environment variable holding the database DSN; never put credentials in arguments")
	expected := flag.String("expected-plan-sha256", "", "Required with --apply: PlanSHA256 from the reviewed dry-run report")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if !*maintenance {
		return fmt.Errorf("refusing database access: stop all game services after normal save, disable automatic restart, then use --maintenance-confirmed")
	}
	if *reportPath == "" {
		return fmt.Errorf("--report is required")
	}
	if *apply && len(*expected) != 64 {
		return fmt.Errorf("--apply requires --expected-plan-sha256 from the reviewed dry-run")
	}
	dsn := os.Getenv(*dsnEnv)
	if dsn == "" {
		return fmt.Errorf("database DSN environment variable is empty")
	}
	out, err := os.OpenFile(*reportPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("cannot create new audit report: %w", err)
	}
	defer out.Close()
	report := showcase.Report{Account: "test001", Status: "not-started"}
	write := func() error {
		b, e := json.MarshalIndent(report, "", "  ")
		if e != nil {
			return e
		}
		if e = out.Truncate(0); e != nil {
			return e
		}
		if _, e = out.Seek(0, 0); e != nil {
			return e
		}
		if _, e = out.Write(append(b, '\n')); e != nil {
			return e
		}
		return out.Sync()
	}
	if err = write(); err != nil {
		return fmt.Errorf("audit report write failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err = store.WithShowcaseMaintenance(ctx, dsn, !*apply, func(tx store.Store, q store.ShowcaseQuerier) error {
		defs, e := data.LoadItems(ctx, q)
		if e != nil {
			return fmt.Errorf("production item catalog load failed: %w", e)
		}
		skills, _, e := data.LoadSkills(ctx, q)
		if e != nil {
			return e
		}
		catalog, e := data.LoadEndgameCatalog(ctx, q, defs, skills.ProgressionCatalog())
		if e != nil {
			return e
		}
		if e = domain.ValidateEquipmentRuntimeV5(skills, catalog); e != nil {
			return e
		}
		domain.AttachEndgameCatalog(defs, catalog)
		if e = store.SetItemDefinitions(tx, defs); e != nil {
			return e
		}
		if e = store.SetEndgameCatalog(tx, catalog); e != nil {
			return e
		}
		if e = domain.ValidateUniqueCatalog(defs, catalog); e != nil {
			return e
		}
		if e = domain.ValidateAstralCatalog(catalog); e != nil {
			return e
		}
		d, e := showcase.PlanDigest(defs)
		if e != nil {
			return e
		}
		if *apply && d != *expected {
			return fmt.Errorf("catalog/plan digest differs from reviewed dry-run")
		}
		warehouseRule, e := data.LoadWarehouseRule(ctx, q)
		if e != nil {
			return e
		}
		report, e = showcase.RunTransaction(ctx, tx, defs, *apply, warehouseRule)
		if e != nil {
			return e
		}
		return write() // A durable precommit audit must exist before commit is allowed.
	})
	if err != nil {
		// Do not print arbitrary driver/server errors: they can include connection
		// strings, usernames or SQL values. Record a bounded diagnostic separately.
		report.Status = "failed-or-commit-unconfirmed; do not retry blindly; inspect fixed receipt with dry-run"
		_ = write()
		return fmt.Errorf("showcase maintenance refused or failed; no success claimed. Keep services stopped and inspect the report; use a dry-run to resolve any uncertain commit. Details: %s", safeError(err, dsn))
	}
	if report.Status == "verified-pending-commit" {
		report.Status = "committed"
	}
	if err = write(); err != nil {
		return fmt.Errorf("database invocation completed, but final report update failed; inspect fixed receipt with dry-run before any retry")
	}
	fmt.Printf("%s: %s\n", report.Status, *reportPath)
	return nil
}

// Intentionally suppress connection errors rather than attempt partial redaction.
func safeError(err error, dsn string) string {
	s := err.Error()
	for _, needle := range []string{"connect showcase", "begin showcase", "commit showcase", "rollback showcase"} {
		if strings.Contains(s, needle) {
			return "database connection/transaction error (credentials suppressed)"
		}
	}
	if strings.Contains(s, dsn) {
		return "database error (credentials suppressed)"
	}
	// Only known tool validation errors are safe for the terminal. Underlying SQL
	// errors are retained neither in report nor terminal.
	for _, needle := range []string{"offline", "other database", "another showcase", "maintenance", "expected", "character ", "receipt", "catalog/plan", "free equipment", "unrelated", "UID", "round-trip", "target", "account missing"} {
		if strings.Contains(s, needle) {
			return s
		}
	}
	return "validation or persistence error (server details suppressed)"
}
