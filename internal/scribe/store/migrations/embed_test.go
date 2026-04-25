package migrations

import (
	"strings"
	"testing"
)

func TestAllReturnsOrderedMigrations(t *testing.T) {
	ms := All()
	if len(ms) == 0 {
		t.Fatal("no migrations registered")
	}
	if ms[0].Version != 1 {
		t.Errorf("first migration version = %d, want 1", ms[0].Version)
	}
	if !strings.Contains(ms[0].SQL, "CREATE TABLE") {
		t.Error("first migration must create tables")
	}
	for i := 1; i < len(ms); i++ {
		if ms[i].Version <= ms[i-1].Version {
			t.Errorf("migrations not strictly ascending at %d", i)
		}
	}
}

func TestInitMigrationDefinesAllTables(t *testing.T) {
	sql := All()[0].SQL
	for _, want := range []string{
		"events", "samples_validator", "samples_chain",
		"state_anchors", "backfill_jobs", "schema_version",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("init migration missing table %q", want)
		}
	}
}
