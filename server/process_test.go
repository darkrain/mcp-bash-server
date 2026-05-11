package server

import (
	"testing"
	"time"
)

func TestProcessRegistryNewAndGet(t *testing.T) {
	r := NewProcessRegistry(1 * time.Minute)
	defer r.Stop()

	p := r.NewProcess("echo hello", "/tmp")
	if p.ID == "" {
		t.Fatal("expected non-empty process ID")
	}
	if p.Status != StatusRunning {
		t.Fatalf("expected running, got %s", p.Status)
	}

	got, ok := r.Get(p.ID)
	if !ok {
		t.Fatal("expected to find process")
	}
	if got.ID != p.ID {
		t.Fatalf("expected ID %s, got %s", p.ID, got.ID)
	}
	if got.Command != "echo hello" {
		t.Fatalf("expected command 'echo hello', got %s", got.Command)
	}
}

func TestProcessRegistryGetNotFound(t *testing.T) {
	r := NewProcessRegistry(1 * time.Minute)
	defer r.Stop()

	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestProcessRegistryUpdate(t *testing.T) {
	r := NewProcessRegistry(1 * time.Minute)
	defer r.Stop()

	p := r.NewProcess("ls", "/")
	r.Update(p.ID, func(proc *Process) {
		proc.Status = StatusCompleted
		proc.ExitCode = 0
	})

	got, _ := r.Get(p.ID)
	if got.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", got.ExitCode)
	}
}

func TestProcessRegistryList(t *testing.T) {
	r := NewProcessRegistry(1 * time.Minute)
	defer r.Stop()

	p1 := r.NewProcess("echo 1", "")
	p2 := r.NewProcess("echo 2", "")

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 processes, got %d", len(list))
	}

	ids := map[string]bool{}
	for _, p := range list {
		ids[p.ID] = true
	}
	if !ids[p1.ID] || !ids[p2.ID] {
		t.Fatal("expected both processes in list")
	}
}

func TestProcessRegistryRemove(t *testing.T) {
	r := NewProcessRegistry(1 * time.Minute)
	defer r.Stop()

	p := r.NewProcess("echo", "")
	r.Remove(p.ID)

	_, ok := r.Get(p.ID)
	if ok {
		t.Fatal("expected process to be removed")
	}
}

func TestProcessRegistryKillAll(t *testing.T) {
	r := NewProcessRegistry(1 * time.Minute)

	p1 := r.NewProcess("sleep 60", "")
	p2 := r.NewProcess("sleep 60", "")

	called1 := false
	called2 := false
	r.Update(p1.ID, func(proc *Process) {
		proc.cancel = func() { called1 = true }
	})
	r.Update(p2.ID, func(proc *Process) {
		proc.cancel = func() { called2 = true }
	})

	r.Stop()

	if !called1 || !called2 {
		t.Fatal("expected cancel to be called for all running processes")
	}
}

func TestProcessRegistryCleanup(t *testing.T) {
	r := NewProcessRegistry(100 * time.Millisecond)

	p := r.NewProcess("echo", "")
	r.Update(p.ID, func(proc *Process) {
		now := time.Now()
		proc.Status = StatusCompleted
		proc.EndedAt = &now
	})

	time.Sleep(250 * time.Millisecond)

	_, ok := r.Get(p.ID)
	if ok {
		t.Fatal("expected completed process to be cleaned up")
	}
}

func TestGenerateProcessID(t *testing.T) {
	ids := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := generateProcessID()
		if len(id) != 16 {
			t.Fatalf("expected 16-char ID, got %d chars: %s", len(id), id)
		}
		if ids[id] {
			t.Fatalf("duplicate ID: %s", id)
		}
		ids[id] = true
	}
}
