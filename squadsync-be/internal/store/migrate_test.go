package store_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"squadsync-be/internal/apptest"
	"squadsync-be/internal/store"
)

func TestMigrateIsSafeToRunAgain(t *testing.T) {
	s, path := apptest.NewStore(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate on the same store: %v", err)
	}

	// A second process (as on every server start) migrates the same file.
	other, err := store.Open(path)
	if err != nil {
		t.Fatalf("open again: %v", err)
	}
	defer other.Close()
	if err := other.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate from a second connection: %v", err)
	}
}

func TestMigrateRefusesTwoFilesWithTheSameVersion(t *testing.T) {
	s, _ := apptest.NewStore(t)
	files := fstest.MapFS{
		"202701010000_add_a.sql": {Data: []byte(`CREATE TABLE test_a (id INTEGER PRIMARY KEY)`)},
		"202701010000_add_b.sql": {Data: []byte(`CREATE TABLE test_b (id INTEGER PRIMARY KEY)`)},
	}

	err := s.MigrateFS(context.Background(), files)
	if err == nil || !strings.Contains(err.Error(), "share version 202701010000") {
		t.Fatalf("MigrateFS with a duplicate version: err = %v, want a 'share version' error", err)
	}
}

func TestMigrateRefusesAVersionAppliedUnderAnotherName(t *testing.T) {
	s, _ := apptest.NewStore(t)
	ctx := context.Background()
	if err := s.MigrateFS(ctx, fstest.MapFS{
		"202701010000_add_a.sql": {Data: []byte(`CREATE TABLE test_a (id INTEGER PRIMARY KEY)`)},
	}); err != nil {
		t.Fatalf("first MigrateFS: %v", err)
	}

	err := s.MigrateFS(ctx, fstest.MapFS{
		"202701010000_renamed.sql": {Data: []byte(`CREATE TABLE test_b (id INTEGER PRIMARY KEY)`)},
	})
	want := "version 202701010000 applied as 202701010000_add_a.sql, file is 202701010000_renamed.sql"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("MigrateFS with a renamed file: err = %v, want it to contain %q", err, want)
	}
}
