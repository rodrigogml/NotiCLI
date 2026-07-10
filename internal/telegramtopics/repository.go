package telegramtopics

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func StatePathForConfig(configPath string) string {
	if strings.TrimSpace(configPath) == "" {
		configPath = filepath.Join("config", "noticli.json")
	}
	dir := filepath.Dir(configPath)
	base := filepath.Base(configPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" || name == "." {
		name = "noticli"
	}
	if ext == "" {
		ext = ".json"
	}
	return filepath.Join(dir, name+".telegram-topics"+ext)
}

type Repository struct {
	path   string
	locker Locker
	now    func() time.Time
}

type RepositoryOption func(*Repository)

type Locker interface {
	WithLock(ctx context.Context, fn func() error) error
}

func NewFileRepository(path string, options ...RepositoryOption) *Repository {
	repository := &Repository{
		path: path,
		now:  func() time.Time { return time.Now().UTC() },
	}
	repository.locker = NewFileLocker(path + ".lock")
	for _, option := range options {
		option(repository)
	}
	return repository
}

func WithClock(now func() time.Time) RepositoryOption {
	return func(repository *Repository) {
		if now != nil {
			repository.now = now
		}
	}
}

func WithLocker(locker Locker) RepositoryOption {
	return func(repository *Repository) {
		if locker != nil {
			repository.locker = locker
		}
	}
}

func (r *Repository) Load(ctx context.Context) (State, error) {
	var state State
	err := r.locker.WithLock(ctx, func() error {
		loaded, err := r.loadLocked()
		if err != nil {
			return err
		}
		state = loaded
		return nil
	})
	if err != nil {
		return State{}, err
	}
	return state, nil
}

func (r *Repository) Save(ctx context.Context, state State) error {
	return r.locker.WithLock(ctx, func() error {
		_, err := r.saveLocked(state)
		return err
	})
}

func (r *Repository) PrepareForUpdate(ctx context.Context) error {
	return r.locker.WithLock(ctx, func() error {
		state, err := r.loadLocked()
		if err != nil {
			return err
		}
		return probeStateWrite(r.path, state)
	})
}

func (r *Repository) Update(ctx context.Context, mutate func(*State) error) (State, error) {
	var updated State
	err := r.locker.WithLock(ctx, func() error {
		state, err := r.loadLocked()
		if err != nil {
			return err
		}
		if mutate != nil {
			if err := mutate(&state); err != nil {
				return err
			}
		}
		state, err = r.saveLocked(state)
		if err != nil {
			return err
		}
		updated = state
		return nil
	})
	if err != nil {
		return State{}, err
	}
	return updated, nil
}

func (r *Repository) loadLocked() (State, error) {
	file, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			state := NewState(r.now())
			if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
				return State{}, fmt.Errorf("telegram topic state directory could not be created: %w", err)
			}
			if err := writeStateAtomically(r.path, state); err != nil {
				return State{}, err
			}
			return state, nil
		}
		return State{}, fmt.Errorf("telegram topic state could not be opened: %w", err)
	}
	defer file.Close()

	state, err := DecodeState(file)
	if err != nil {
		return State{}, err
	}
	return state, nil
}

func (r *Repository) saveLocked(state State) (State, error) {
	now := r.now().UTC()
	state.Version = StateVersion
	state.UpdatedAt = now
	if state.Associations == nil {
		state.Associations = []Association{}
	}

	current, exists, err := r.readExistingState()
	if err != nil {
		return State{}, err
	}
	if exists {
		if err := backupState(r.BackupPath(), current); err != nil {
			return State{}, err
		}
		backupAt := now
		state.PreviousBackupAt = &backupAt
	}
	if err := writeStateAtomically(r.path, state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (r *Repository) readExistingState() (State, bool, error) {
	file, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, false, nil
		}
		return State{}, false, fmt.Errorf("telegram topic state could not be opened: %w", err)
	}
	defer file.Close()

	state, err := DecodeState(file)
	if err != nil {
		return State{}, true, err
	}
	return state, true, nil
}

func (r *Repository) BackupPath() string {
	return r.path + ".bak"
}

func backupState(path string, state State) error {
	return writeStateAtomically(path, state)
}

func writeStateAtomically(path string, state State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("telegram topic state directory could not be created: %w", err)
	}

	var buffer bytes.Buffer
	if err := EncodeState(&buffer, state); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("telegram topic state temp file could not be created: %w", err)
	}
	tempPath := tempFile.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(tempFile, &buffer); err != nil {
		tempFile.Close()
		return fmt.Errorf("telegram topic state temp file could not be written: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("telegram topic state temp file could not be closed: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("telegram topic state could not be replaced: %w", err)
	}
	removeTemp = false
	return nil
}

func probeStateWrite(path string, state State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("telegram topic state directory could not be created: %w", err)
	}

	var buffer bytes.Buffer
	if err := EncodeState(&buffer, state); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".probe-*")
	if err != nil {
		return fmt.Errorf("telegram topic state write permission could not be verified: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := io.Copy(tempFile, &buffer); err != nil {
		tempFile.Close()
		return fmt.Errorf("telegram topic state write permission could not be verified: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("telegram topic state write permission could not be verified: %w", err)
	}
	return nil
}

type FileLocker struct {
	path          string
	retryInterval time.Duration
}

func NewFileLocker(path string) *FileLocker {
	return &FileLocker{
		path:          path,
		retryInterval: 10 * time.Millisecond,
	}
}

func (l *FileLocker) WithLock(ctx context.Context, fn func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("telegram topic state lock directory could not be created: %w", err)
	}

	file, err := l.acquire(ctx)
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		os.Remove(l.path)
	}()

	if fn == nil {
		return nil
	}
	return fn()
}

func (l *FileLocker) acquire(ctx context.Context) (*os.File, error) {
	interval := l.retryInterval
	if interval <= 0 {
		interval = 10 * time.Millisecond
	}
	for {
		file, err := os.OpenFile(l.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if _, writeErr := fmt.Fprintf(file, "%d\n", os.Getpid()); writeErr != nil {
				file.Close()
				os.Remove(l.path)
				return nil, fmt.Errorf("telegram topic state lock could not be written: %w", writeErr)
			}
			return file, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("telegram topic state lock could not be acquired: %w", err)
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("telegram topic state lock wait canceled: %w", ctx.Err())
		case <-timer.C:
		}
	}
}
