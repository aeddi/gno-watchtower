package migrations

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var fs embed.FS

// Migration is a numbered DDL bundle.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// All returns the embedded migrations sorted by version ascending.
func All() []Migration {
	entries, err := fs.ReadDir(".")
	if err != nil {
		panic(fmt.Errorf("read embedded migrations: %w", err))
	}
	var ms []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".sql")
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			panic(fmt.Errorf("malformed migration filename %q", e.Name()))
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			panic(fmt.Errorf("non-numeric prefix in %q: %w", e.Name(), err))
		}
		body, err := fs.ReadFile(e.Name())
		if err != nil {
			panic(err)
		}
		ms = append(ms, Migration{Version: v, Name: parts[1], SQL: string(body)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].Version < ms[j].Version })
	return ms
}
