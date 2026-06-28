package telegramtopics_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rodrigogml/NotiCLI/internal/telegramtopics"
)

func TestStatePathForConfigUsesSiblingFile(t *testing.T) {
	got := telegramtopics.StatePathForConfig(filepath.Join("opt", "NotiCLI", "config", "noticli.json"))
	want := filepath.Join("opt", "NotiCLI", "config", "noticli.telegram-topics.json")
	if got != want {
		t.Fatalf("StatePathForConfig() = %q, want %q", got, want)
	}
}

func TestStatePathForConfigHandlesEmptyPath(t *testing.T) {
	got := telegramtopics.StatePathForConfig("")
	want := "noticli.telegram-topics.json"
	if got != want {
		t.Fatalf("StatePathForConfig(\"\") = %q, want %q", got, want)
	}
}

func TestRepositoryLoadInitializesMissingState(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "telegram-topics.json")
	repository := telegramtopics.NewFileRepository(path, telegramtopics.WithClock(func() time.Time { return now }))

	state, err := repository.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if state.Version != telegramtopics.StateVersion {
		t.Fatalf("Version = %d, want %d", state.Version, telegramtopics.StateVersion)
	}
	if !state.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want %s", state.UpdatedAt, now)
	}
	if state.Associations == nil || len(state.Associations) != 0 {
		t.Fatalf("Associations = %#v, want empty slice", state.Associations)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file was not initialized: %v", err)
	}
}

func TestRepositoryRejectsMalformedStateWithoutOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telegram-topics.json")
	original := []byte(`{"version":`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	repository := telegramtopics.NewFileRepository(path)

	if _, err := repository.Load(context.Background()); err == nil {
		t.Fatal("Load() error = nil, want malformed state error")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != string(original) {
		t.Fatalf("state file was overwritten: %s", string(data))
	}
}

func TestRepositorySaveWritesAtomicallyAndBacksUpPreviousValidState(t *testing.T) {
	firstTime := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	secondTime := firstTime.Add(time.Minute)
	times := []time.Time{firstTime, secondTime}
	path := filepath.Join(t.TempDir(), "telegram-topics.json")
	repository := telegramtopics.NewFileRepository(path, telegramtopics.WithClock(func() time.Time {
		now := times[0]
		times = times[1:]
		return now
	}))

	if err := repository.Save(context.Background(), telegramtopics.State{
		Version:      telegramtopics.StateVersion,
		UpdatedAt:    firstTime,
		Associations: []telegramtopics.Association{validAssociation("ops", "-100", "BackupJob", 4, firstTime)},
	}); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if err := repository.Save(context.Background(), telegramtopics.State{
		Version:      telegramtopics.StateVersion,
		UpdatedAt:    secondTime,
		Associations: []telegramtopics.Association{validAssociation("ops", "-100", "BillingJob", 5, secondTime)},
	}); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	state, err := repository.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(state.Associations) != 1 || state.Associations[0].Sender != "BillingJob" {
		t.Fatalf("current state associations = %#v", state.Associations)
	}
	if state.PreviousBackupAt == nil || !state.PreviousBackupAt.Equal(secondTime) {
		t.Fatalf("PreviousBackupAt = %v, want %s", state.PreviousBackupAt, secondTime)
	}

	backupData, err := os.Open(repository.BackupPath())
	if err != nil {
		t.Fatalf("Open(backup) error = %v", err)
	}
	defer backupData.Close()
	backup, err := telegramtopics.DecodeState(backupData)
	if err != nil {
		t.Fatalf("DecodeState(backup) error = %v", err)
	}
	if len(backup.Associations) != 1 || backup.Associations[0].Sender != "BackupJob" {
		t.Fatalf("backup associations = %#v", backup.Associations)
	}
}

func TestRepositoryUsesInjectedLockerAdapter(t *testing.T) {
	locker := &countingLocker{}
	path := filepath.Join(t.TempDir(), "telegram-topics.json")
	repository := telegramtopics.NewFileRepository(path, telegramtopics.WithLocker(locker))

	if _, err := repository.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if locker.calls != 1 {
		t.Fatalf("locker calls = %d, want 1", locker.calls)
	}
}

func TestRepositorySerializesConcurrentUpdates(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "telegram-topics.json")
	repository := telegramtopics.NewFileRepository(path, telegramtopics.WithClock(func() time.Time { return now }))

	const workers = 10
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repository.Update(context.Background(), func(state *telegramtopics.State) error {
				state.Associations = append(state.Associations, validAssociation("ops", "-100", senderName(i), i+1, now))
				return nil
			})
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}
	}

	state, err := repository.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(state.Associations) != workers {
		t.Fatalf("Associations length = %d, want %d", len(state.Associations), workers)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestFileLockerHonorsContextCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telegram-topics.json.lock")
	held, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile(lock) error = %v", err)
	}
	defer held.Close()
	defer os.Remove(path)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	locker := telegramtopics.NewFileLocker(path)
	if err := locker.WithLock(ctx, func() error { return nil }); err == nil {
		t.Fatal("WithLock() error = nil, want context cancellation")
	}
}

func TestRepositoryLockFailureDoesNotCorruptExistingState(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "telegram-topics.json")
	repository := telegramtopics.NewFileRepository(path, telegramtopics.WithClock(func() time.Time { return now }))
	original := telegramtopics.State{
		Version:      telegramtopics.StateVersion,
		UpdatedAt:    now,
		Associations: []telegramtopics.Association{validAssociation("ops", "-100", "BackupJob", 4, now)},
	}
	if err := repository.Save(context.Background(), original); err != nil {
		t.Fatalf("Save(original) error = %v", err)
	}

	lockPath := path + ".lock"
	held, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile(lock) error = %v", err)
	}
	defer held.Close()
	defer os.Remove(lockPath)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	err = repository.Save(ctx, telegramtopics.State{
		Version:      telegramtopics.StateVersion,
		UpdatedAt:    now,
		Associations: []telegramtopics.Association{validAssociation("ops", "-100", "BillingJob", 5, now)},
	})
	if err == nil {
		t.Fatal("Save() error = nil, want lock timeout")
	}

	held.Close()
	os.Remove(lockPath)
	state, err := repository.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(state.Associations) != 1 || state.Associations[0].Sender != "BackupJob" {
		t.Fatalf("state after lock failure = %#v, want original association", state.Associations)
	}
}

type countingLocker struct {
	calls int
}

func (l *countingLocker) WithLock(_ context.Context, fn func() error) error {
	l.calls++
	return fn()
}

func validAssociation(recipientID, chatID, sender string, threadID int, now time.Time) telegramtopics.Association {
	return telegramtopics.Association{
		RecipientID:      recipientID,
		ChatID:           chatID,
		Sender:           sender,
		TopicName:        sender,
		MessageThreadID:  threadID,
		CreatedByNotiCLI: true,
		CreatedAt:        now,
		Status:           telegramtopics.TopicStatusActive,
	}
}

func senderName(index int) string {
	return "Sender" + string(rune('A'+index))
}
