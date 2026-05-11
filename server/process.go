package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"

	bolt "go.etcd.io/bbolt"
)

type ProcessStatus string

const (
	StatusRunning   ProcessStatus = "running"
	StatusCompleted ProcessStatus = "completed"
	StatusFailed    ProcessStatus = "failed"
	StatusKilled    ProcessStatus = "killed"
)

var bucketName = []byte("processes")

type Process struct {
	ID         string        `json:"id"`
	Command    string        `json:"command"`
	Cwd        string        `json:"cwd,omitempty"`
	PID        int           `json:"pid"`
	Status     ProcessStatus `json:"status"`
	ExitCode   int           `json:"exit_code,omitempty"`
	StartedAt  time.Time     `json:"started_at"`
	EndedAt    *time.Time    `json:"ended_at,omitempty"`
	Duration   int64         `json:"duration_ms,omitempty"`
	OutputPath string        `json:"output_path"`
}

type ProcessRegistry struct {
	db     *bolt.DB
	dir    string
	ttl    time.Duration
	stopCh chan struct{}
	logger *slog.Logger
}

func NewProcessRegistry(dir string, ttl time.Duration, logger *slog.Logger) (*ProcessRegistry, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create process dir: %w", err)
	}
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}

	dbPath := filepath.Join(dir, "processes.db")
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open bbolt: %w", err)
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	r := &ProcessRegistry{
		db:     db,
		dir:    dir,
		ttl:    ttl,
		stopCh: make(chan struct{}),
		logger: logger,
	}

	r.recover()

	if ttl > 0 {
		go r.cleanupLoop()
	}

	return r, nil
}

func (r *ProcessRegistry) recover() {
	var toUpdate []Process

	r.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var p Process
			if err := json.Unmarshal(v, &p); err != nil {
				continue
			}
			if p.Status != StatusRunning {
				continue
			}
			if !isPIDAlive(p.PID) {
				p.Status = StatusFailed
				p.ExitCode = -1
				now := time.Now()
				p.EndedAt = &now
				toUpdate = append(toUpdate, p)
			}
		}
		return nil
	})

	for _, p := range toUpdate {
		r.save(p)
		if r.logger != nil {
			r.logger.Info("recovered stale process", "process_id", p.ID, "pid", p.PID, "status", string(p.Status))
		}
	}
}

func (r *ProcessRegistry) NewProcess(command, cwd string) *Process {
	id := generateProcessID()
	outputPath := filepath.Join(r.dir, "output", id+".log")

	p := &Process{
		ID:         id,
		Command:    command,
		Cwd:        cwd,
		Status:     StatusRunning,
		StartedAt:  time.Now(),
		OutputPath: outputPath,
	}

	r.save(*p)
	return p
}

func (r *ProcessRegistry) Get(id string) (*Process, bool) {
	var p Process
	found := false

	r.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		v := b.Get([]byte(id))
		if v == nil {
			return nil
		}
		if err := json.Unmarshal(v, &p); err != nil {
			return nil
		}
		found = true
		return nil
	})

	if !found {
		return nil, false
	}
	return &p, true
}

func (r *ProcessRegistry) Update(id string, fn func(p *Process)) {
	r.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		v := b.Get([]byte(id))
		if v == nil {
			return nil
		}
		var p Process
		if err := json.Unmarshal(v, &p); err != nil {
			return nil
		}
		fn(&p)
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return b.Put([]byte(id), data)
	})
}

func (r *ProcessRegistry) List() []*Process {
	var result []*Process

	r.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var p Process
			if err := json.Unmarshal(v, &p); err != nil {
				continue
			}
			result = append(result, &p)
		}
		return nil
	})

	return result
}

func (r *ProcessRegistry) Remove(id string) {
	r.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		v := b.Get([]byte(id))
		if v != nil {
			var p Process
			if json.Unmarshal(v, &p) == nil {
				os.Remove(p.OutputPath)
			}
		}
		return b.Delete([]byte(id))
	})
}

func (r *ProcessRegistry) CreateOutputFile(id string) (*os.File, error) {
	path := filepath.Join(r.dir, "output", id+".log")
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}

func (r *ProcessRegistry) ReadOutput(p *Process, maxSize int) (string, error) {
	f, err := os.Open(p.OutputPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}

	size := info.Size()
	if maxSize > 0 && size > int64(maxSize) {
		offset := size - int64(maxSize)
		_, err = f.Seek(offset, 0)
		if err != nil {
			return "", err
		}
		buf := make([]byte, maxSize)
		n, err := f.Read(buf)
		if err != nil {
			return "", err
		}
		return "... [output truncated]\n" + string(buf[:n]), nil
	}

	buf := make([]byte, size)
	_, err = f.ReadAt(buf, 0)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func (r *ProcessRegistry) Stop() {
	close(r.stopCh)
	r.db.Close()
}

func (r *ProcessRegistry) save(p Process) {
	r.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return b.Put([]byte(p.ID), data)
	})
}

func (r *ProcessRegistry) cleanupLoop() {
	interval := r.ttl / 2
	if interval > time.Minute {
		interval = time.Minute
	}

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
	var toDelete []string
	now := time.Now()

	r.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var p Process
			if err := json.Unmarshal(v, &p); err != nil {
				continue
			}
			if p.Status == StatusRunning {
				continue
			}
			if p.EndedAt != nil && now.Sub(*p.EndedAt) > r.ttl {
				toDelete = append(toDelete, p.ID)
			}
		}
		return nil
	})

	for _, id := range toDelete {
		r.Remove(id)
	}
}

func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func generateProcessID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
