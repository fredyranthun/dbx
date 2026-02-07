package session

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// SessionState represents the lifecycle state of a forwarding session.
type SessionState string

const (
	SessionStateStarting SessionState = "starting"
	SessionStateRunning  SessionState = "running"
	SessionStateStopping SessionState = "stopping"
	SessionStateStopped  SessionState = "stopped"
	SessionStateError    SessionState = "error"
)

// SessionKey identifies a session by service/env.
type SessionKey string

func NewSessionKey(service, env string) SessionKey {
	return SessionKey(fmt.Sprintf("%s/%s", service, env))
}

func (k SessionKey) String() string {
	return string(k)
}

// Session tracks process metadata, status, and log streaming state.
type Session struct {
	Key     SessionKey
	Service string
	Env     string

	Bind      string
	LocalPort int

	RemoteHost       string
	RemotePort       int
	TargetInstanceID string
	Region           string
	Profile          string

	PID       int
	State     SessionState
	StartTime time.Time
	LastError string

	cmd    *exec.Cmd
	cancel context.CancelFunc

	logBuf *RingBuffer

	subsMu           sync.RWMutex
	subscribers      map[uint64]chan string
	nextSubscriberID uint64
}

func NewSession(service, env string) *Session {
	return &Session{
		Key:         NewSessionKey(service, env),
		Service:     service,
		Env:         env,
		State:       SessionStateStarting,
		logBuf:      NewRingBuffer(DefaultRingBufferLines),
		subscribers: make(map[uint64]chan string),
	}
}

func (s *Session) ensureLogState() {
	if s.logBuf == nil {
		s.logBuf = NewRingBuffer(DefaultRingBufferLines)
	}
	if s.subscribers == nil {
		s.subscribers = make(map[uint64]chan string)
	}
}

// AppendLog appends a line to the ring buffer and broadcasts to subscribers.
func (s *Session) AppendLog(line string) {
	if s == nil {
		return
	}

	s.subsMu.Lock()
	defer s.subsMu.Unlock()

	s.ensureLogState()
	s.logBuf.Append(line)
	for _, ch := range s.subscribers {
		select {
		case ch <- line:
		default:
		}
	}
}

func (s *Session) LastLogs(n int) []string {
	if s == nil {
		return nil
	}

	s.subsMu.RLock()
	defer s.subsMu.RUnlock()

	if s.logBuf == nil {
		return nil
	}
	return s.logBuf.Last(n)
}

// SubscribeLogs registers a subscriber channel for follow mode.
func (s *Session) SubscribeLogs(buffer int) (uint64, <-chan string) {
	if s == nil {
		ch := make(chan string)
		close(ch)
		return 0, ch
	}
	if buffer < 0 {
		buffer = 0
	}

	s.subsMu.Lock()
	defer s.subsMu.Unlock()

	s.ensureLogState()
	s.nextSubscriberID++
	id := s.nextSubscriberID
	ch := make(chan string, buffer)
	s.subscribers[id] = ch

	return id, ch
}

func (s *Session) UnsubscribeLogs(id uint64) {
	if s == nil {
		return
	}

	s.subsMu.Lock()
	defer s.subsMu.Unlock()

	ch, ok := s.subscribers[id]
	if !ok {
		return
	}
	delete(s.subscribers, id)
	close(ch)
}

func (s *Session) CloseLogSubscribers() {
	if s == nil {
		return
	}

	s.subsMu.Lock()
	defer s.subsMu.Unlock()

	for id, ch := range s.subscribers {
		delete(s.subscribers, id)
		close(ch)
	}
}
