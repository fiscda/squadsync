package store_test

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"squadsync-be/internal/store"
)

// rebuiltByVendeg are the tables 202611290101_vendeg_foreign_keys.sql
// rebuilds to add REFERENCES vendeg (id), with how many rows the test puts
// in each.
var rebuiltByVendeg = []string{
	"visszajelzes", "tetel_vallalas", "auto_utas", "sajat_uton",
	"agy_foglalas", "mashol_alszik", "kiadas_resz", "elszamolas_megerosites",
}

// migrationsBefore returns the real migration files older than version.
func migrationsBefore(t *testing.T, version string) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	names, err := fs.Glob(os.DirFS("migrations"), "*.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	for _, name := range names {
		if name >= version {
			continue
		}
		body, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		out[name] = &fstest.MapFile{Data: body}
	}
	return out
}

func TestTheVendegMigrationsKeepTheRowsOfTheRebuiltTables(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.MigrateFS(ctx, migrationsBefore(t, "202611290100")); err != nil {
		t.Fatalf("migrate to before the vendeg migrations: %v", err)
	}

	// A database as #13 finds it: a row in every claim table.
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer db.Close()
	const at = "2026-03-02T09:00:00.000000000Z"
	for _, stmt := range []string{
		`INSERT INTO felhasznalo (id, name, created_at) VALUES ('anna', 'Anna', '` + at + `')`,
		`INSERT INTO csoport (id, name, created_at) VALUES ('c', 'Péntek esték', '` + at + `')`,
		`INSERT INTO esemeny (id, csoport_id, title, description, helyszin_name, helyszin_url, szervezo_id, allapot, created_at)
		 VALUES ('e', 'c', 'Túra', '', '', '', 'anna', 'egyeztetes', '` + at + `')`,
		`INSERT INTO idopont (id, esemeny_id, start_at, added_by, created_at) VALUES ('i', 'e', '` + at + `', 'anna', '` + at + `')`,
		`INSERT INTO visszajelzes (idopont_id, felhasznalo_id, valasz, created_at, updated_at) VALUES ('i', 'anna', 'igen', '` + at + `', '` + at + `')`,
		`INSERT INTO tetel (id, esemeny_id, name, added_by, created_at) VALUES ('t', 'e', 'Sátor', 'anna', '` + at + `')`,
		`INSERT INTO tetel_vallalas (tetel_id, felhasznalo_id, quantity, created_at, updated_at) VALUES ('t', 'anna', 1, '` + at + `', '` + at + `')`,
		`INSERT INTO auto (id, esemeny_id, sofor_id, seats, created_at) VALUES ('a', 'e', 'anna', 3, '` + at + `')`,
		`INSERT INTO auto_utas (auto_id, esemeny_id, felhasznalo_id, created_at) VALUES ('a', 'e', 'anna', '` + at + `')`,
		`INSERT INTO sajat_uton (esemeny_id, felhasznalo_id, created_at) VALUES ('e', 'anna', '` + at + `')`,
		`INSERT INTO szoba (id, esemeny_id, name, position, created_at) VALUES ('sz', 'e', 'Nappali', 0, '` + at + `')`,
		`INSERT INTO agy (id, szoba_id, esemeny_id, capacity, position, created_at) VALUES ('ag', 'sz', 'e', 2, 0, '` + at + `')`,
		`INSERT INTO agy_foglalas (agy_id, esemeny_id, felhasznalo_id, created_at) VALUES ('ag', 'e', 'anna', '` + at + `')`,
		`INSERT INTO mashol_alszik (esemeny_id, felhasznalo_id, created_at) VALUES ('e', 'anna', '` + at + `')`,
		`INSERT INTO kiadas (id, esemeny_id, description, amount, payer_id, allapot, egyenlo, created_by, created_at, updated_at)
		 VALUES ('k', 'e', 'Bor', 900, 'anna', 'kifizetve', 1, 'anna', '` + at + `', '` + at + `')`,
		`INSERT INTO kiadas_resz (kiadas_id, felhasznalo_id, amount) VALUES ('k', 'anna', 900)`,
		`INSERT INTO elszamolas_megerosites (esemeny_id, felhasznalo_id, megerositette, created_at) VALUES ('e', 'anna', 'anna', '` + at + `')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", strings.Fields(stmt)[2], err)
		}
	}

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate with the vendeg migrations: %v", err)
	}

	for _, table := range rebuiltByVendeg {
		var n int
		var felhasznalo string
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*), MAX(felhasznalo_id) FROM `+table).Scan(&n, &felhasznalo); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 1 || felhasznalo != "anna" {
			t.Fatalf("%s after the rebuild: %d rows of %q, want Anna's one", table, n, felhasznalo)
		}
		// The rebuilt table names the vendeg table in a foreign key.
		var fk int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pragma_foreign_key_list(?) WHERE "table" = 'vendeg' AND "from" = 'vendeg_id'`, table,
		).Scan(&fk); err != nil {
			t.Fatalf("foreign keys of %s: %v", table, err)
		}
		if fk != 1 {
			t.Fatalf("%s has no foreign key from vendeg_id to vendeg", table)
		}
	}
	// The indexes are back: one Visszajelzes per Idopont and Felhasznalo.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO visszajelzes (idopont_id, felhasznalo_id, valasz, created_at, updated_at) VALUES ('i', 'anna', 'nem', ?, ?)`, at, at,
	); err == nil {
		t.Fatalf("a second visszajelzes of Anna on the same Idopont was accepted")
	}
	// And an unknown Vendeg is refused now.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO visszajelzes (idopont_id, vendeg_id, valasz, created_at, updated_at) VALUES ('i', 'ghost', 'igen', ?, ?)`, at, at,
	); err == nil {
		t.Fatalf("a visszajelzes of an unknown vendeg was accepted")
	}
	var problems int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&problems); err != nil || problems != 0 {
		t.Fatalf("foreign key check: %d problems, %v", problems, err)
	}
}
