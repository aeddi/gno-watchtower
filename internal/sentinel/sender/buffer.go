// internal/sentinel/sender/buffer.go
package sender

import "sync"

// Buffer is a thread-safe in-memory FIFO queue with a max size.
// When full, Push drops the oldest item and returns dropped=true.
type Buffer[T any] struct {
	mu      sync.Mutex
	items   []T
	maxSize int
}

func NewBuffer[T any](maxSize int) *Buffer[T] {
	if maxSize <= 0 {
		panic("sender.NewBuffer: maxSize must be > 0")
	}
	return &Buffer[T]{
		items:   make([]T, 0, maxSize),
		maxSize: maxSize,
	}
}

// Push adds item to the buffer. If the buffer is at capacity, the oldest item is dropped.
func (b *Buffer[T]) Push(item T) (dropped bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) >= b.maxSize {
		b.items = b.items[1:]
		dropped = true
	}
	b.items = append(b.items, item)
	return dropped
}

// Drain removes and returns all items. Returns nil if the buffer is empty.
func (b *Buffer[T]) Drain() []T {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) == 0 {
		return nil
	}
	out := make([]T, len(b.items))
	copy(out, b.items)
	b.items = b.items[:0:0]
	return out
}

func (b *Buffer[T]) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}
