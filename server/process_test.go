package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func newTestRegistry(t *testing.T) *ProcessRegistry {
	t.Helper()
	dir := t.TempDir()
	r, err := NewProcessRegistry(dir, 60*time.Minute, nil)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	t.Cleanup(func() { r.Stop() })
	return r
}

func TestProcessRegistryNewAndGet(t *testing.T) {
	r := newTestRegistry(t)

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
	r := newTestRegistry(t)

	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestProcessRegistryUpdate(t *testing.T) {
	r := newTestRegistry(t)

	p := r.NewProcess("ls", "/")
	r.Update(p.ID, func(proc *Process) {
		proc.Status = StatusCompleted
		proc.ExitCode = 0
		proc.PID = 123
	})

	got, _ := r.Get(p.ID)
	if got.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", got.ExitCode)
	}
	if got.PID != 123 {
		t.Fatalf("expected PID 123, got %d", got.PID)
	}
}

func TestProcessRegistryList(t *testing.T) {
	r := newTestRegistry(t)

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
	r := newTestRegistry(t)

	p := r.NewProcess("echo", "")
	outPath := filepath.Join(r.dir, "output", p.ID+".log")
	os.WriteFile(outPath, []byte("test"), 0644)

	r.Remove(p.ID)

	_, ok := r.Get(p.ID)
	if ok {
		t.Fatal("expected process to be removed")
	}

	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatal("expected output file to be removed")
	}
}

func TestProcessRegistryStop(t *testing.T) {
	dir := t.TempDir()
	r, err := NewProcessRegistry(dir, 60*time.Minute, nil)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	p := r.NewProcess("sleep 60", "")
	r.Update(p.ID, func(proc *Process) {
		proc.PID = 999999
	})

	r.Stop()

	_, err = os.Stat(filepath.Join(dir, "processes.db"))
	if os.IsNotExist(err) {
		t.Fatal("expected db file to persist after stop")
	}
}

func TestProcessRegistryCleanup(t *testing.T) {
	dir := t.TempDir()
	r, err := NewProcessRegistry(dir, 200*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	defer r.Stop()

	p := r.NewProcess("echo", "")
	r.Update(p.ID, func(proc *Process) {
		now := time.Now()
		proc.Status = StatusCompleted
		proc.EndedAt = &now
	})

	time.Sleep(500 * time.Millisecond)

	_, ok := r.Get(p.ID)
	if ok {
		t.Fatal("expected completed process to be cleaned up")
	}
}

func TestProcessRegistryRecovery(t *testing.T) {
	dir := t.TempDir()

	r1, err := NewProcessRegistry(dir, 60*time.Minute, nil)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	p := r1.NewProcess("echo hello", "/tmp")
	r1.Update(p.ID, func(proc *Process) {
		proc.PID = 99999999
	})
	r1.Stop()

	r2, err := NewProcessRegistry(dir, 60*time.Minute, nil)
	if err != nil {
		t.Fatalf("failed to create registry on recovery: %v", err)
	}
	defer r2.Stop()

	got, ok := r2.Get(p.ID)
	if !ok {
		t.Fatal("expected to find process after recovery")
	}
	if got.Status != StatusFailed {
		t.Fatalf("expected failed (dead PID), got %s", got.Status)
	}
	if got.PID != 99999999 {
		t.Fatalf("expected PID 99999999, got %d", got.PID)
	}
}

func TestProcessRegistryOutputFile(t *testing.T) {
	r := newTestRegistry(t)

	p := r.NewProcess("echo test", "/")
	f, err := r.CreateOutputFile(p.ID)
	if err != nil {
		t.Fatalf("failed to create output file: %v", err)
	}
	f.WriteString("hello output\n")
	f.Close()

	content, err := r.ReadOutput(p, 0)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if content != "hello output\n" {
		t.Fatalf("expected 'hello output\\n', got %q", content)
	}
}

func TestProcessRegistryOutputTruncate(t *testing.T) {
	r := newTestRegistry(t)

	p := r.NewProcess("big output", "")
	f, err := r.CreateOutputFile(p.ID)
	if err != nil {
		t.Fatalf("failed to create output file: %v", err)
	}
	longStr := string(make([]byte, 2048))
	f.WriteString(longStr)
	f.Close()

	content, err := r.ReadOutput(p, 1024)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if len(content) > 1100 {
		t.Fatalf("expected truncated output <= ~1100 chars, got %d", len(content))
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

func TestProcessRegistryPersistence(t *testing.T) {
	dir := t.TempDir()

	r1, err := NewProcessRegistry(dir, 60*time.Minute, nil)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	p := r1.NewProcess("ls -la", "/home")
	r1.Update(p.ID, func(proc *Process) {
		now := time.Now()
		proc.Status = StatusCompleted
		proc.ExitCode = 0
		proc.PID = 1234
		proc.EndedAt = &now
		proc.Duration = now.Sub(proc.StartedAt).Milliseconds()
	})
	r1.Stop()

	r2, err := NewProcessRegistry(dir, 60*time.Minute, nil)
	if err != nil {
		t.Fatalf("failed to create registry on recovery: %v", err)
	}
	defer r2.Stop()

	got, ok := r2.Get(p.ID)
	if !ok {
		t.Fatal("expected to find process after recovery")
	}
	if got.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", got.ExitCode)
	}
	if got.PID != 1234 {
		t.Fatalf("expected PID 1234, got %d", got.PID)
	}
	if got.Command != "ls -la" {
		t.Fatalf("expected command 'ls -la', got %s", got.Command)
	}
	if got.Cwd != "/home" {
		t.Fatalf("expected cwd '/home', got %s", got.Cwd)
	}

	var rawData []byte
	r2.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		rawData = b.Get([]byte(p.ID))
		return nil
	})

	var raw Process
	if err := json.Unmarshal(rawData, &raw); err != nil {
		t.Fatalf("failed to unmarshal stored data: %v", err)
	}
	if raw.OutputPath != got.OutputPath {
		t.Fatalf("expected output_path %s, got %s", raw.OutputPath, got.OutputPath)
	}
}
