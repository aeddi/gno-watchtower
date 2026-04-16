// internal/sentinel/sender/buffer_test.go
package sender_test

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/sentinel/sender"
)

func TestBuffer_PushAndDrain(t *testing.T) {
	b := sender.NewBuffer[int](5)
	for i := 0; i < 3; i++ {
		dropped := b.Push(i)
		if dropped {
			t.Errorf("Push(%d): unexpected drop", i)
		}
	}
	if b.Len() != 3 {
		t.Fatalf("Len: got %d, want 3", b.Len())
	}

	items := b.Drain()
	if len(items) != 3 {
		t.Fatalf("Drain: got %d items, want 3", len(items))
	}
	for i, v := range items {
		if v != i {
			t.Errorf("items[%d]: got %d, want %d", i, v, i)
		}
	}
	if b.Len() != 0 {
		t.Errorf("Len after Drain: got %d, want 0", b.Len())
	}
}

func TestBuffer_DropOldestWhenFull(t *testing.T) {
	b := sender.NewBuffer[int](3)
	b.Push(1)
	b.Push(2)
	b.Push(3)

	dropped := b.Push(4)
	if !dropped {
		t.Fatal("expected drop=true when buffer full")
	}
	if b.Len() != 3 {
		t.Errorf("Len after overflow: got %d, want 3", b.Len())
	}

	items := b.Drain()
	if len(items) != 3 {
		t.Fatalf("Drain: got %d items, want 3", len(items))
	}
	// Oldest (1) should have been dropped; items should be [2, 3, 4].
	if items[0] != 2 || items[1] != 3 || items[2] != 4 {
		t.Errorf("unexpected items after overflow: %v", items)
	}
}

func TestBuffer_ConcurrentSafe(t *testing.T) {
	b := sender.NewBuffer[int](10)
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(i int) {
			b.Push(i)
			done <- struct{}{}
		}(i)
	}
	go func() {
		b.Drain()
		done <- struct{}{}
	}()
	for i := 0; i < 11; i++ {
		<-done
	}
}

func TestBuffer_DrainEmpty(t *testing.T) {
	b := sender.NewBuffer[string](10)
	items := b.Drain()
	if items != nil {
		t.Errorf("Drain empty: got %v, want nil", items)
	}
}
