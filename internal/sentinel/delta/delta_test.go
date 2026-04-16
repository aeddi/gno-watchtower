// internal/sentinel/delta/delta_test.go
package delta_test

import (
	"fmt"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/sentinel/delta"
)

func TestDelta_FirstCallAlwaysChanged(t *testing.T) {
	d := delta.NewDelta()
	if !d.Changed("status", []byte(`{"height":"1"}`)) {
		t.Error("first call should always return changed=true")
	}
}

func TestDelta_UnchangedDataReturnsFalse(t *testing.T) {
	d := delta.NewDelta()
	raw := []byte(`{"height":"1"}`)
	d.Changed("status", raw)
	if d.Changed("status", raw) {
		t.Error("same data should return changed=false")
	}
}

func TestDelta_ChangedDataReturnsTrue(t *testing.T) {
	d := delta.NewDelta()
	d.Changed("status", []byte(`{"height":"1"}`))
	if !d.Changed("status", []byte(`{"height":"2"}`)) {
		t.Error("different data should return changed=true")
	}
}

func TestDelta_IndependentKeysAreTrackedSeparately(t *testing.T) {
	d := delta.NewDelta()
	raw := []byte(`{"v":1}`)
	d.Changed("a", raw)
	d.Changed("b", raw)
	if d.Changed("a", raw) {
		t.Error("key 'a' should be unchanged")
	}
	if d.Changed("b", raw) {
		t.Error("key 'b' should be unchanged")
	}
}

func TestDelta_ConcurrentSafe(t *testing.T) {
	d := delta.NewDelta()
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(i int) {
			raw := []byte(fmt.Sprintf(`{"i":%d}`, i))
			d.Changed("key", raw)
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
