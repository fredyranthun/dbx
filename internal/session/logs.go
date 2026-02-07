package session

import (
	"fmt"
	"sync"
)

const DefaultRingBufferLines = 500

// RingBuffer stores log lines in a fixed-size circular buffer.
type RingBuffer struct {
	mu    sync.RWMutex
	buf   []string
	head  int
	count int
}

// NewRingBuffer creates a ring buffer; non-positive capacity uses the default.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultRingBufferLines
	}

	return &RingBuffer{buf: make([]string, capacity)}
}

// Append stores one line, evicting the oldest line when full.
func (r *RingBuffer) Append(line string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.buf) == 0 {
		return
	}

	r.buf[r.head] = line
	r.head = (r.head + 1) % len(r.buf)
	if r.count < len(r.buf) {
		r.count++
	}
}

// Last returns the last n lines ordered from oldest to newest.
func (r *RingBuffer) Last(n int) []string {
	if r == nil || n <= 0 {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.count == 0 {
		return nil
	}
	if n > r.count {
		n = r.count
	}

	start := (r.head - n + len(r.buf)) % len(r.buf)
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		idx := (start + i) % len(r.buf)
		out = append(out, r.buf[idx])
	}

	return out
}

// LastLogs returns the last n log lines for a session key.
func (m *Manager) LastLogs(key SessionKey, n int) ([]string, error) {
	if m == nil {
		return nil, fmt.Errorf("manager is nil")
	}

	m.mu.RLock()
	s, ok := m.sessions[key]
	m.mu.RUnlock()
	if !ok || s == nil {
		return nil, fmt.Errorf("%s: session not found", key)
	}

	return s.LastLogs(n), nil
}

// SubscribeLogs subscribes to streaming logs for the given session key.
func (m *Manager) SubscribeLogs(key SessionKey, buffer int) (uint64, <-chan string, error) {
	if m == nil {
		return 0, nil, fmt.Errorf("manager is nil")
	}

	m.mu.RLock()
	s, ok := m.sessions[key]
	m.mu.RUnlock()
	if !ok || s == nil {
		return 0, nil, fmt.Errorf("%s: session not found", key)
	}

	id, ch := s.SubscribeLogs(buffer)
	return id, ch, nil
}

// UnsubscribeLogs detaches a prior log subscription. Missing sessions are ignored.
func (m *Manager) UnsubscribeLogs(key SessionKey, id uint64) {
	if m == nil || id == 0 {
		return
	}

	m.mu.RLock()
	s, ok := m.sessions[key]
	m.mu.RUnlock()
	if !ok || s == nil {
		return
	}

	s.UnsubscribeLogs(id)
}
