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
		"--recipient", "ops",
		"--channel", "email",
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
	if request.RecipientID != "ops" {
		t.Fatalf("RecipientID = %q", request.RecipientID)
	}
	if request.Channel != notify.ChannelEmail {
		t.Fatalf("Channel = %q", request.Channel)
	}
	if request.Title != "Backup failed" {
		t.Fatalf("Title = %q", request.Title)
	}
	if request.Message != "Nightly backup failed" {
		t.Fatalf("Message = %q", request.Message)
	}
	if len(request.Attachments) != 2 {
		t.Fatalf("Attachments length = %d", len(request.Attachments))
	}
	if request.Attachments[0].Path != "./a.txt" || request.Attachments[1].Path != "./b.txt" {
		t.Fatalf("Attachments = %#v", request.Attachments)
	}
}

func TestParseSendDefaultsConfigPathToExecutableDirectory(t *testing.T) {
	executablePath := filepath.Join(t.TempDir(), "bin", "noticli")

	request, err := cli.ParseWithExecutablePath([]string{
		"send",
		"--sender", "BackupJob",
		"--recipient", "ops",
		"--channel", "email",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, executablePath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := filepath.Join(filepath.Dir(executablePath), cli.DefaultConfigFileName)
	if request.ConfigPath != want {
		t.Fatalf("ConfigPath = %q, want %q", request.ConfigPath, want)
	}
}

func TestDefaultConfigPathPrefersPublishedConfigSymlink(t *testing.T) {
	root := t.TempDir()

	configDir := filepath.Join(root, "opt", "NotiCLI", "config")
	binDir := filepath.Join(root, "opt", "NotiCLI", "bin")
	releaseDir := filepath.Join(root, "opt", "NotiCLI", "releases", "v1.1.0")
	usrLocalBinDir := filepath.Join(root, "usr", "local", "bin")

	for _, dir := range []string{configDir, binDir, releaseDir, usrLocalBinDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
	}

	configPath := filepath.Join(configDir, "noticli.json")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	if err := os.Symlink(filepath.Join(releaseDir, "noticli"), filepath.Join(binDir, "noticli")); err != nil {
		t.Fatalf("Symlink(bin) error = %v", err)
	}
	if err := os.Symlink(filepath.Join(binDir, "noticli"), filepath.Join(usrLocalBinDir, "noticli")); err != nil {
		t.Fatalf("Symlink(usrlocal) error = %v", err)
	}

	got := cli.DefaultConfigPath(filepath.Join(usrLocalBinDir, "noticli"))
	want, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(config) error = %v", err)
	}
	if got != want {
		t.Fatalf("DefaultConfigPath() = %q, want %q", got, want)
	}
}

func TestParseSendRejectsEmptyExplicitConfigPath(t *testing.T) {
	_, err := cli.ParseWithExecutablePath([]string{
		"send",
		"--config", "",
		"--sender", "BackupJob",
		"--recipient", "ops",
		"--channel", "email",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, filepath.Join(t.TempDir(), "noticli"))
	if err == nil {
		t.Fatal("Parse() error = nil, want empty config error")
	}
}

func TestParseSendRejectsMissingSenderSystem(t *testing.T) {
	_, err := cli.ParseWithExecutablePath([]string{
		"send",
		"--recipient", "ops",
		"--channel", "email",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, filepath.Join(t.TempDir(), "noticli"))
	if err == nil {
		t.Fatal("Parse() error = nil, want missing sender error")
	}
}

func TestParseSendRejectsLongSenderSystem(t *testing.T) {
	_, err := cli.ParseWithExecutablePath([]string{
		"send",
		"--sender", "SystemNameLongerThan20",
		"--recipient", "ops",
		"--channel", "email",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, filepath.Join(t.TempDir(), "noticli"))
	if err == nil {
		t.Fatal("Parse() error = nil, want long sender error")
	}
}

func TestParseSendRejectsMissingRequiredFlags(t *testing.T) {
	_, err := cli.ParseWithExecutablePath([]string{
		"send",
		"--sender", "BackupJob",
		"--channel", "email",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, filepath.Join(t.TempDir(), "noticli"))
	if err == nil {
		t.Fatal("Parse() error = nil, want missing recipient error")
	}
}

func TestParseSendRejectsUnsupportedChannel(t *testing.T) {
	_, err := cli.ParseWithExecutablePath([]string{
		"send",
		"--sender", "BackupJob",
		"--recipient", "ops",
		"--channel", "sms",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, filepath.Join(t.TempDir(), "noticli"))
	if err == nil {
		t.Fatal("Parse() error = nil, want unsupported channel error")
	}
}

func TestRunIsNonInteractiveAndReturnsInvalidInputForParseErrors(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.Run([]string{"send", "--sender", "BackupJob", "--recipient", "ops"}, &stdout, &stderr)
	if code != cli.ExitInvalidInput {
		t.Fatalf("Run() exit code = %d, want %d", code, cli.ExitInvalidInput)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() == "" {
		t.Fatal("stderr is empty, want diagnostic")
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
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	got := stderr.String()
	for _, want := range []string{
		"invalid_input: missing command",
		"Usage:",
		"noticli send --sender <system>",
		"Required flags:",
		"Examples:",
	} {
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
		"--recipient", "ops",
		"--channel", "email",
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
		"--recipient", "ops",
		"--channel", "email",
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
		"--recipient", "ops",
		"--channel", "telegram",
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
		"recipients": {
			"ops": {"email": "ops@example.com"}
		},
		"channels": {
			"email": {
				"settings": {"from": "noticli@example.com"},
				"secrets": {"smtp_password": "secret"},
				"attachments": "supported"
			}
		}
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
		"recipients": {
			"ops": {"email": "ops@example.com", "telegram_chat_id": "12345"}
		},
		"channels": {
			"telegram": {
				"settings": {},
				"secrets": {"token": "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ"},
				"attachments": "limited"
			}
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
