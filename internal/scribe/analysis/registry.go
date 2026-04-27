package analysis

import (
	"fmt"
	"sort"
	"sync"
)

// registryMu guards the global registry. Rules register in init() — sequential,
// no race in practice — but tests reset state between cases, so we lock.
var registryMu sync.RWMutex

// registry is keyed by Meta.Kind() (e.g. "diagnostic.block_missed_v1") and
// stores the rule + its embedded markdown doc.
var registry = map[string]registryEntry{}

type registryEntry struct {
	rule Rule
	meta Meta
	doc  string
}

// Register adds a rule and its embedded markdown doc. Called from init() in
// each rule file. Panics on duplicate code+version or empty doc — these are
// programmer errors that must fail at startup, not silently.
func Register(r Rule, doc string) {
	if r == nil {
		panic("analysis.Register: nil rule")
	}
	if doc == "" {
		panic(fmt.Sprintf("analysis.Register: rule %s has empty doc — every rule MUST embed its markdown", r.Meta().Kind()))
	}
	m := r.Meta()
	key := m.Kind()
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[key]; exists {
		panic(fmt.Sprintf("analysis.Register: %s already registered", key))
	}
	registry[key] = registryEntry{rule: r, meta: m, doc: doc}
}

// RegisteredCodes returns every registered rule's Kind, sorted lexically.
// Used by /api/rules and CI guards.
func RegisteredCodes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// GetDoc returns the embedded markdown for a registered rule, or "" if absent.
func GetDoc(kind string) string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[kind].doc
}

// GetMeta returns the registered Meta or a zero-value Meta if absent.
func GetMeta(kind string) Meta {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[kind].meta
}

// Lookup returns the registered Rule or nil.
func Lookup(kind string) Rule {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[kind].rule
}

// ResetRegistryForTest is a test-only helper exported so tests in other
// packages (e.g. internal/scribe/api) can isolate registry state.
func ResetRegistryForTest(t interface {
	Helper()
	Cleanup(func())
},
) {
	t.Helper()
	registryMu.Lock()
	prev := registry
	registry = map[string]registryEntry{}
	registryMu.Unlock()
	t.Cleanup(func() {
		registryMu.Lock()
		registry = prev
		registryMu.Unlock()
	})
}

// resetRegistryForTest is a package-internal alias used by analysis tests.
func resetRegistryForTest(t interface {
	Helper()
	Cleanup(func())
},
) {
	ResetRegistryForTest(t)
}
