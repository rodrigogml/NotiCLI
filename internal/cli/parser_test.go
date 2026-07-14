package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/cli"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

func TestParseSendMapsFlagsToNotificationRequest(t *testing.T) {
	request, err := cli.ParseWithExecutablePath([]string{
		"send",
		"--config", "./noticli.json",
		"--sender", "BackupJob",
		"--category", "backup",
		"--priority", "high",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
		"--attach", "./a.txt",
		"--attach", "./b.txt",
	}, filepath.Join(t.TempDir(), "noticli"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if request.ConfigPath != "./noticli.json" {
		t.Fatalf("ConfigPath = %q", request.ConfigPath)
	}
	if request.SenderSystem != "BackupJob" {
		t.Fatalf("SenderSystem = %q", request.SenderSystem)
	}
	if request.Category != "backup" {
		t.Fatalf("Category = %q", request.Category)
	}
	if request.Priority != notify.PriorityHigh {
		t.Fatalf("Priority = %q", request.Priority)
	}
	if request.Title != "Backup failed" || request.Message != "Nightly backup failed" {
		t.Fatalf("request title/message = %#v", request)
	}
	if len(request.Attachments) != 2 {
		t.Fatalf("Attachments length = %d", len(request.Attachments))
	}
}

func TestParseSendDefaultsPriorityAndConfigPath(t *testing.T) {
	executablePath := filepath.Join(t.TempDir(), "bin", "noticli")
	request, err := cli.ParseWithExecutablePath([]string{
		"send",
		"--sender", "BackupJob",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, executablePath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := filepath.Join(filepath.Dir(executablePath), "config", cli.DefaultConfigFileName)
	if request.ConfigPath != want {
		t.Fatalf("ConfigPath = %q, want %q", request.ConfigPath, want)
	}
	if request.Priority != notify.PriorityNormal {
		t.Fatalf("Priority = %q, want NORMAL", request.Priority)
	}
}

func TestParseWatchTelegramBOTMapsFlags(t *testing.T) {
	request, err := cli.ParseWatchTelegramBOTWithExecutablePath([]string{
		"--config", "./noticli.json",
		"--account", "telegram-main",
		"--poll-seconds", "5",
		"--max-seconds", "60",
		"--log", "./tmp/events.jsonl",
	}, filepath.Join(t.TempDir(), "noticli"))
	if err != nil {
		t.Fatalf("ParseWatchTelegramBOT() error = %v", err)
	}

	if request.ConfigPath != "./noticli.json" {
		t.Fatalf("ConfigPath = %q", request.ConfigPath)
	}
	if request.AccountID != "telegram-main" {
		t.Fatalf("AccountID = %q", request.AccountID)
	}
	if request.PollSeconds != 5 {
		t.Fatalf("PollSeconds = %d", request.PollSeconds)
	}
	if request.MaxSeconds != 60 {
		t.Fatalf("MaxSeconds = %d", request.MaxSeconds)
	}
	if request.LogPath != "./tmp/events.jsonl" {
		t.Fatalf("LogPath = %q", request.LogPath)
	}
}

func TestParseWatchTelegramBOTDefaultsConfigAndRuntimeFlags(t *testing.T) {
	executablePath := filepath.Join(t.TempDir(), "bin", "noticli")
	request, err := cli.ParseWatchTelegramBOTWithExecutablePath(nil, executablePath)
	if err != nil {
		t.Fatalf("ParseWatchTelegramBOT() error = %v", err)
	}

	wantConfig := filepath.Join(filepath.Dir(executablePath), "config", cli.DefaultConfigFileName)
	if request.ConfigPath != wantConfig {
		t.Fatalf("ConfigPath = %q, want %q", request.ConfigPath, wantConfig)
	}
	if request.PollSeconds != 3 {
		t.Fatalf("PollSeconds = %d, want 3", request.PollSeconds)
	}
	if request.MaxSeconds != 0 {
		t.Fatalf("MaxSeconds = %d, want 0", request.MaxSeconds)
	}
	if request.LogPath != cli.DefaultWatchLogPath {
		t.Fatalf("LogPath = %q, want %q", request.LogPath, cli.DefaultWatchLogPath)
	}
}

func TestParseWatchTelegramBOTRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "empty config", args: []string{"--config", ""}},
		{name: "empty account", args: []string{"--account", ""}},
		{name: "zero poll", args: []string{"--poll-seconds", "0"}},
		{name: "negative poll", args: []string{"--poll-seconds", "-1"}},
		{name: "negative max", args: []string{"--max-seconds", "-1"}},
		{name: "empty log", args: []string{"--log", ""}},
		{name: "unexpected argument", args: []string{"extra"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cli.ParseWatchTelegramBOTWithExecutablePath(tt.args, filepath.Join(t.TempDir(), "noticli"))
			if err == nil {
				t.Fatal("ParseWatchTelegramBOT() error = nil, want error")
			}
		})
	}
}

func TestDefaultConfigPathUsesReleaseConfigDirectoryOnly(t *testing.T) {
	root := t.TempDir()

	binDir := filepath.Join(root, "opt", "NotiCLI", "bin")
	releaseDir := filepath.Join(root, "opt", "NotiCLI", "releases", "v1.1.2")
	usrLocalBinDir := filepath.Join(root, "usr", "local", "bin")

	for _, dir := range []string{binDir, releaseDir, usrLocalBinDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
	}

	if err := os.Symlink(filepath.Join(releaseDir, "noticli"), filepath.Join(binDir, "noticli")); err != nil {
		t.Fatalf("Symlink(bin) error = %v", err)
	}
	if err := os.Symlink(filepath.Join(binDir, "noticli"), filepath.Join(usrLocalBinDir, "noticli")); err != nil {
		t.Fatalf("Symlink(usrlocal) error = %v", err)
	}

	got := cli.DefaultConfigPath(filepath.Join(usrLocalBinDir, "noticli"))
	want := filepath.Join(releaseDir, "config", cli.DefaultConfigFileName)
	if got != want {
		t.Fatalf("DefaultConfigPath() = %q, want %q", got, want)
	}
}

func TestDefaultConfigPathDoesNotUseCallerPathWhenExecutablePathIsUnavailable(t *testing.T) {
	got := cli.DefaultConfigPath("")
	want := filepath.Join("config", cli.DefaultConfigFileName)
	if got != want {
		t.Fatalf("DefaultConfigPath() = %q, want %q", got, want)
	}
}

func TestParseSendRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "empty config", args: []string{"send", "--config", "", "--sender", "BackupJob", "--title", "Title", "--message", "Body"}},
		{name: "missing sender", args: []string{"send", "--title", "Title", "--message", "Body"}},
		{name: "long sender", args: []string{"send", "--sender", "SystemNameLongerThan20", "--title", "Title", "--message", "Body"}},
		{name: "missing title", args: []string{"send", "--sender", "BackupJob", "--message", "Body"}},
		{name: "invalid priority", args: []string{"send", "--sender", "BackupJob", "--priority", "URGENT", "--title", "Title", "--message", "Body"}},
		{name: "old recipient flag", args: []string{"send", "--sender", "BackupJob", "--recipient", "ops", "--title", "Title", "--message", "Body"}},
		{name: "old channel flag", args: []string{"send", "--sender", "BackupJob", "--channel", "email", "--title", "Title", "--message", "Body"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cli.ParseWithExecutablePath(tt.args, filepath.Join(t.TempDir(), "noticli"))
			if err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
		})
	}
}

func TestRunIsNonInteractiveAndReturnsInvalidInputForParseErrors(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.Run([]string{"send", "--sender", "BackupJob"}, &stdout, &stderr)
	if code != cli.ExitInvalidInput {
		t.Fatalf("Run() exit code = %d, want %d", code, cli.ExitInvalidInput)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("stderr = %q, want usage help", stderr.String())
	}
}

func TestRunWithoutArgumentsReturnsUsageHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.Run(nil, &stdout, &stderr)
	if code != cli.ExitInvalidInput {
		t.Fatalf("Run() exit code = %d, want %d", code, cli.ExitInvalidInput)
	}
	got := stderr.String()
	for _, want := range []string{"invalid_input: missing command", "Usage:", "noticli send --sender <system>", "Required flags:", "Examples:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunLoadsConfigFromConfigFlag(t *testing.T) {
	configPath := writeConfig(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.Run([]string{
		"send",
		"--config", configPath,
		"--sender", "BackupJob",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, &stdout, &stderr)
	if code != 4 {
		t.Fatalf("Run() exit code = %d, want invalid_config code 4", code)
	}
	if got := stderr.String(); got != "invalid_config: email: required setting \"host\" is missing\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRunReturnsMissingConfigWhenConfigFileDoesNotExist(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.Run([]string{
		"send",
		"--config", filepath.Join(t.TempDir(), "missing.json"),
		"--sender", "BackupJob",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("Run() exit code = %d, want missing_config code 3", code)
	}
	if got := stderr.String(); got == "" {
		t.Fatal("stderr is empty, want diagnostic")
	}
}

func TestRunReturnsInvalidConfigWithoutLeakingSecrets(t *testing.T) {
	configPath := writeInvalidTelegramConfig(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.Run([]string{
		"send",
		"--config", configPath,
		"--sender", "BackupJob",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, &stdout, &stderr)
	if code != 4 {
		t.Fatalf("Run() exit code = %d, want invalid_config code 4", code)
	}
	got := stderr.String()
	if strings.Contains(got, "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Fatalf("stderr leaked secret: %q", got)
	}
	if !strings.Contains(got, "invalid_config: telegram:") {
		t.Fatalf("stderr = %q, want telegram invalid_config", got)
	}
}

func writeConfig(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "noticli.json")
	content := `{
		"destinations": {
			"ops-email": {"type": "email", "email": "ops@example.com"}
		},
		"delivery_accounts": {
			"smtp-main": {
				"type": "email",
				"settings": {"from": "noticli@example.com"},
				"secrets": {"smtp_password": "secret"},
				"attachments": "supported"
			}
		},
		"catch_all": {"deliveries": [{"account": "smtp-main", "destination": "ops-email"}]}
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeInvalidTelegramConfig(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "noticli.json")
	content := `{
		"destinations": {
			"ops-telegram": {"type": "telegram", "telegram_chat_id": "12345"}
		},
		"delivery_accounts": {
			"telegram-main": {
				"type": "telegram",
				"settings": {},
				"secrets": {"token": "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ"},
				"attachments": "limited"
			}
		},
		"catch_all": {"deliveries": [{"account": "telegram-main", "destination": "ops-telegram"}]}
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
