package store_test

import (
	"context"
	"testing"

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
