package server

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

type ProcessStatus string

const (
	StatusRunning   ProcessStatus = "running"
	StatusCompleted ProcessStatus = "completed"
	StatusFailed    ProcessStatus = "failed"
	StatusKilled    ProcessStatus = "killed"
)

type Process struct {
	ID        string        `json:"id"`
	Command   string        `json:"command"`
	Cwd       string        `json:"cwd,omitempty"`
	Status    ProcessStatus `json:"status"`
	ExitCode  int           `json:"exit_code,omitempty"`
	Stdout    string        `json:"stdout,omitempty"`
	Stderr    string        `json:"stderr,omitempty"`
	StartedAt time.Time     `json:"started_at"`
	EndedAt   *time.Time    `json:"ended_at,omitempty"`
	Duration  int64         `json:"duration_ms,omitempty"`
	cancel    context.CancelFunc
}

type ProcessRegistry struct {
	mu        sync.RWMutex
	processes map[string]*Process
	ttl       time.Duration
	stopCh    chan struct{}
}

func NewProcessRegistry(ttl time.Duration) *ProcessRegistry {
	r := &ProcessRegistry{
		processes: make(map[string]*Process),
		ttl:       ttl,
		stopCh:    make(chan struct{}),
	}
	if ttl > 0 {
		interval := ttl / 2
		if interval > time.Minute {
			interval = time.Minute
		}
		go r.cleanupLoop(interval)
	}
	return r
}

func (r *ProcessRegistry) Stop() {
	close(r.stopCh)
	r.killAll()
}

func (r *ProcessRegistry) killAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.processes {
		if p.Status == StatusRunning && p.cancel != nil {
			p.cancel()
		}
	}
}

func (r *ProcessRegistry) NewProcess(command, cwd string) *Process {
	id := generateProcessID()
	p := &Process{
		ID:        id,
		Command:   command,
		Cwd:       cwd,
		Status:    StatusRunning,
		StartedAt: time.Now(),
	}
	r.mu.Lock()
	r.processes[id] = p
	r.mu.Unlock()
	return p
}

func (r *ProcessRegistry) Get(id string) (*Process, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.processes[id]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}

func (r *ProcessRegistry) Update(id string, fn func(p *Process)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.processes[id]; ok {
		fn(p)
	}
}

func (r *ProcessRegistry) List() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Process, 0, len(r.processes))
	for _, p := range r.processes {
		cp := *p
		result = append(result, &cp)
	}
	return result
}

func (r *ProcessRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.processes, id)
}

func (r *ProcessRegistry) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.cleanup()
		}
	}
}

func (r *ProcessRegistry) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for id, p := range r.processes {
		if p.Status == StatusRunning {
			continue
		}
		if p.EndedAt != nil && now.Sub(*p.EndedAt) > r.ttl {
			delete(r.processes, id)
		}
	}
}

func generateProcessID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
