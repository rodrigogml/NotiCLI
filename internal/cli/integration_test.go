package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/cli"
	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

func TestRunWithSendersHappyPathByChannel(t *testing.T) {
	configPath := writeIntegrationConfig(t)

	tests := []struct {
		name    string
		channel string
		wantOut string
	}{
		{name: "email", channel: notify.ChannelEmail, wantOut: "email accepted\n"},
		{name: "telegram", channel: notify.ChannelTelegram, wantOut: "telegram accepted\n"},
		{name: "slack", channel: notify.ChannelSlack, wantOut: "slack accepted\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeChannelSender{
				name:   tt.channel,
				result: notify.SuccessResult(tt.channel, tt.wantOut[:len(tt.wantOut)-1]),
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := cli.RunWithSenders(sendArgs(configPath, tt.channel), &stdout, &stderr, sender)
			if code != diagnostics.ExitSuccess {
				t.Fatalf("RunWithSenders() exit code = %d, want 0; stderr=%q", code, stderr.String())
			}
			if got := stdout.String(); got != tt.wantOut {
				t.Fatalf("stdout = %q, want %q", got, tt.wantOut)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if sender.calls != 1 {
				t.Fatalf("sender calls = %d, want 1", sender.calls)
			}
			if sender.request.Channel != tt.channel {
				t.Fatalf("request channel = %q, want %q", sender.request.Channel, tt.channel)
			}
		})
	}
}

func TestRunWithSendersReturnsExpectedExitCodesForFailureScenarios(t *testing.T) {
	configPath := writeIntegrationConfig(t)
	missingAttachment := filepath.Join(t.TempDir(), "missing.txt")

	tests := []struct {
		name       string
		args       []string
		senders    []notify.ChannelSender
		wantCode   int
		wantStderr string
	}{
		{
			name:       "invalid input",
			args:       []string{"send", "--sender", "BackupJob", "--recipient", "ops"},
			wantCode:   diagnostics.ExitInvalidInput,
			wantStderr: "invalid_input:",
		},
		{
			name:       "missing config",
			args:       sendArgs(filepath.Join(t.TempDir(), "missing.json"), notify.ChannelEmail),
			wantCode:   diagnostics.ExitMissingConfig,
			wantStderr: "missing_config:",
		},
		{
			name:       "invalid config",
			args:       sendArgs(writeInvalidIntegrationConfig(t), notify.ChannelEmail),
			wantCode:   diagnostics.ExitInvalidConfig,
			wantStderr: "invalid_config:",
		},
		{
			name:       "attachment error",
			args:       append(sendArgs(configPath, notify.ChannelEmail), "--attach", missingAttachment),
			senders:    []notify.ChannelSender{&fakeChannelSender{name: notify.ChannelEmail, result: notify.SuccessResult(notify.ChannelEmail, "accepted")}},
			wantCode:   diagnostics.ExitAttachmentError,
			wantStderr: "attachment_error:",
		},
		{
			name: "delivery failure",
			args: sendArgs(configPath, notify.ChannelSlack),
			senders: []notify.ChannelSender{&fakeChannelSender{
				name: notify.ChannelSlack,
				err:  diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelSlack, "provider rejected request"),
			}},
			wantCode:   diagnostics.ExitDeliveryFailure,
			wantStderr: "delivery_failure: slack:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := cli.RunWithSenders(tt.args, &stdout, &stderr, tt.senders...)
			if code != tt.wantCode {
				t.Fatalf("RunWithSenders() exit code = %d, want %d; stderr=%q", code, tt.wantCode, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRunWithSendersRedactsConfiguredSecretsFromChannelDiagnostics(t *testing.T) {
	configPath := writeIntegrationConfig(t)
	sender := &fakeChannelSender{
		name: notify.ChannelSlack,
		err: diagnostics.ForChannel(
			diagnostics.CategoryDeliveryFailure,
			notify.ChannelSlack,
			"provider rejected webhook https://hooks.slack.com/services/T000/B000/secret token=123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ password=SMTP_PASSWORD_PLACEHOLDER",
		),
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.RunWithSenders(sendArgs(configPath, notify.ChannelSlack), &stdout, &stderr, sender)
	if code != diagnostics.ExitDeliveryFailure {
		t.Fatalf("RunWithSenders() exit code = %d, want %d", code, diagnostics.ExitDeliveryFailure)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	got := stderr.String()
	if !strings.Contains(got, "delivery_failure: slack:") {
		t.Fatalf("stderr = %q, want channel diagnostic", got)
	}
	for _, leaked := range []string{
		"https://hooks.slack.com/services/T000/B000/secret",
		"123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ",
		"SMTP_PASSWORD_PLACEHOLDER",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("stderr leaked %q in %q", leaked, got)
		}
	}
	if strings.Count(got, diagnostics.Redacted) < 3 {
		t.Fatalf("stderr = %q, want redacted secrets", got)
	}
}

func sendArgs(configPath, channel string) []string {
	return []string{
		"send",
		"--config", configPath,
		"--sender", "TestRunner",
		"--recipient", "ops",
		"--channel", channel,
		"--title", "Test",
		"--message", "Integration test",
	}
}

type fakeChannelSender struct {
	name    string
	result  notify.Result
	err     error
	calls   int
	request notify.Request
}

func (f *fakeChannelSender) Name() string {
	return f.name
}

func (f *fakeChannelSender) Send(_ context.Context, request notify.Request, _ notify.Recipient, _ notify.ChannelConfig) (notify.Result, error) {
	f.calls++
	f.request = request
	return f.result, f.err
}

func writeIntegrationConfig(t *testing.T) string {
	t.Helper()

	return writeFile(t, "noticli.json", `{
		"recipients": {
			"ops": {
				"email": "ops@example.invalid",
				"telegram_chat_id": "12345",
				"slack_destination": "#ops"
			}
		},
		"channels": {
			"email": {
				"settings": {
					"host": "smtp.example.invalid",
					"port": "587",
					"from": "noticli@example.invalid"
				},
				"secrets": {"smtp_password": "SMTP_PASSWORD_PLACEHOLDER"},
				"attachments": "supported"
			},
			"telegram": {
				"settings": {"parse_mode": "HTML"},
				"secrets": {"token": "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ"},
				"attachments": "unsupported"
			},
			"slack": {
				"settings": {"workspace": "ops"},
				"secrets": {"webhook_url": "https://hooks.slack.com/services/T000/B000/secret"},
				"attachments": "unsupported"
			}
		}
	}`)
}

func writeInvalidIntegrationConfig(t *testing.T) string {
	t.Helper()

	return writeFile(t, "invalid-noticli.json", `{"recipients": {}, "channels": {}}`)
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
