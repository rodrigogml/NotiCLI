package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rodrigogml/NotiCLI/internal/channels/telegram"
	"github.com/rodrigogml/NotiCLI/internal/cli"
	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
	"github.com/rodrigogml/NotiCLI/internal/telegramtopics"
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

func TestRunWithSendersTelegramPrivateAndTopicsEndToEnd(t *testing.T) {
	var createCalls int
	var sendPayloads []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot123456:ABCDEF/createForumTopic":
			createCalls++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":4,"name":"BackupJob"}}`))
		case "/bot123456:ABCDEF/sendMessage":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode(send) error = %v", err)
			}
			sendPayloads = append(sendPayloads, payload)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	configPath := writeTelegramIntegrationConfig(t, `"private": {"telegram_chat_id": "12345"}, "topics": {"telegram_delivery_mode": "topics", "telegram_topic_group_chat_id": "-1001234567890"}`)
	repository := telegramtopics.NewFileRepository(filepath.Join(t.TempDir(), "telegram-topics.json"))
	sender := telegram.NewSender(server.Client(), telegram.WithBaseURL(server.URL), telegram.WithTopicStore(repository))

	for _, recipient := range []string{"private", "topics", "topics"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		args := []string{
			"send",
			"--config", configPath,
			"--sender", "BackupJob",
			"--recipient", recipient,
			"--channel", notify.ChannelTelegram,
			"--title", "Backup failed",
			"--message", "Nightly backup failed",
		}
		code := cli.RunWithSenders(args, &stdout, &stderr, sender)
		if code != diagnostics.ExitSuccess {
			t.Fatalf("RunWithSenders(%s) exit code = %d, want 0; stderr=%q", recipient, code, stderr.String())
		}
	}

	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want one creation for two topic sends", createCalls)
	}
	if len(sendPayloads) != 3 {
		t.Fatalf("sendPayloads length = %d, want 3", len(sendPayloads))
	}
	if sendPayloads[0]["text"] != "[BackupJob] Backup failed\n\nNightly backup failed" {
		t.Fatalf("private text = %q", sendPayloads[0]["text"])
	}
	if _, ok := sendPayloads[0]["message_thread_id"]; ok {
		t.Fatalf("private payload has thread ID: %#v", sendPayloads[0])
	}
	for index, payload := range sendPayloads[1:] {
		if payload["text"] != "Backup failed\n\nNightly backup failed" {
			t.Fatalf("topic text %d = %q", index, payload["text"])
		}
		if payload["message_thread_id"] != float64(4) {
			t.Fatalf("topic thread %d = %#v, want 4", index, payload["message_thread_id"])
		}
	}
}

func TestRunWithSendersTelegramTopicsIncompleteConfiguration(t *testing.T) {
	configPath := writeTelegramIntegrationConfig(t, `"topics": {"telegram_delivery_mode": "topics"}`)
	sender := telegram.NewSender(failingHTTPClient{})
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.RunWithSenders([]string{
		"send",
		"--config", configPath,
		"--sender", "BackupJob",
		"--recipient", "topics",
		"--channel", notify.ChannelTelegram,
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, &stdout, &stderr, sender)
	if code != diagnostics.ExitInvalidConfig {
		t.Fatalf("RunWithSenders() exit code = %d, want invalid_config; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "invalid_config: telegram:") {
		t.Fatalf("stderr = %q, want telegram invalid_config", stderr.String())
	}
}

func TestRunWithSendersTelegramStaleTopicRecovery(t *testing.T) {
	var sendCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot123456:ABCDEF/sendMessage":
			sendCalls++
			if sendCalls == 1 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"ok":false,"description":"Bad Request: message thread not found"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		case "/bot123456:ABCDEF/createForumTopic":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":6,"name":"BackupJob"}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	configPath := writeTelegramIntegrationConfig(t, `"topics": {"telegram_delivery_mode": "topics", "telegram_topic_group_chat_id": "-1001234567890"}`)
	repository := telegramtopics.NewFileRepository(filepath.Join(t.TempDir(), "telegram-topics.json"))
	now := "2026-06-28T12:00:00Z"
	if err := repository.Save(context.Background(), telegramtopics.State{
		Version:   telegramtopics.StateVersion,
		UpdatedAt: mustParseTime(t, now),
		Associations: []telegramtopics.Association{
			{
				RecipientID:      "topics",
				ChatID:           "-1001234567890",
				Sender:           "BackupJob",
				TopicName:        "BackupJob",
				MessageThreadID:  4,
				CreatedByNotiCLI: true,
				CreatedAt:        mustParseTime(t, now),
				Status:           telegramtopics.TopicStatusActive,
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	sender := telegram.NewSender(server.Client(), telegram.WithBaseURL(server.URL), telegram.WithTopicStore(repository))
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.RunWithSenders([]string{
		"send",
		"--config", configPath,
		"--sender", "BackupJob",
		"--recipient", "topics",
		"--channel", notify.ChannelTelegram,
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	}, &stdout, &stderr, sender)
	if code != diagnostics.ExitSuccess {
		t.Fatalf("RunWithSenders() exit code = %d, want success; stderr=%q", code, stderr.String())
	}
	if sendCalls != 2 {
		t.Fatalf("sendCalls = %d, want original send and retry", sendCalls)
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

type failingHTTPClient struct{}

func (failingHTTPClient) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected HTTP call")
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

func writeTelegramIntegrationConfig(t *testing.T, recipients string) string {
	t.Helper()

	return writeFile(t, "telegram-noticli.json", `{
		"recipients": {`+recipients+`},
		"channels": {
			"telegram": {
				"settings": {"parse_mode": "HTML"},
				"secrets": {"token": "123456:ABCDEF"},
				"attachments": "unsupported"
			}
		}
	}`)
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", value, err)
	}
	return parsed
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
