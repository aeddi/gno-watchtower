package handlers

import (
	"fmt"
	"sort"
	"sync"
)

// Factory builds a fresh Handler for a given cluster ID. Each handler kind
// registers a Factory in init().
type Factory func(clusterID string) Handler

type handlerEntry struct {
	factory Factory
	doc     string
}

var (
	handlerMu  sync.RWMutex
	handlerReg = map[string]handlerEntry{}
)

// Register adds a handler kind to the registry. Called from init() per kind.
// Panics on duplicate kind or empty doc — programmer errors must fail loud.
func Register(kind string, f Factory, doc string) {
	if doc == "" {
		panic(fmt.Sprintf("handlers.Register: kind %s has empty doc", kind))
	}
	handlerMu.Lock()
	defer handlerMu.Unlock()
	if _, exists := handlerReg[kind]; exists {
		panic(fmt.Sprintf("handlers.Register: %s already registered", kind))
	}
	handlerReg[kind] = handlerEntry{factory: f, doc: doc}
}

// RegisteredKinds returns every registered handler kind, sorted lexically.
func RegisteredKinds() []string {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	out := make([]string, 0, len(handlerReg))
	for k := range handlerReg {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// GetHandlerDoc returns the embedded markdown for a registered kind or "".
func GetHandlerDoc(kind string) string {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	return handlerReg[kind].doc
}

// NewHandler instantiates a handler for the given kind + cluster, or returns
// nil if the kind is unregistered.
func NewHandler(kind, cluster string) Handler {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	e, ok := handlerReg[kind]
	if !ok {
		return nil
	}
	return e.factory(cluster)
}

// resetHandlerRegistryForTest is a test-only helper. Reseeds registry state
// at test start; restores at test cleanup.
func resetHandlerRegistryForTest(t interface {
	Helper()
	Cleanup(func())
},
) {
	t.Helper()
	handlerMu.Lock()
	prev := handlerReg
	handlerReg = map[string]handlerEntry{}
	handlerMu.Unlock()
	t.Cleanup(func() {
		handlerMu.Lock()
		handlerReg = prev
		handlerMu.Unlock()
	})
}
