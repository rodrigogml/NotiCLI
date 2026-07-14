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

	"github.com/rodrigogml/NotiCLI/internal/channels/telegram"
	"github.com/rodrigogml/NotiCLI/internal/cli"
	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
	"github.com/rodrigogml/NotiCLI/internal/telegramtopics"
)

func TestRunWithSendersBroadcastsMatchingRoutes(t *testing.T) {
	configPath := writeIntegrationConfig(t)
	emailSender := &fakeChannelSender{name: notify.ChannelEmail, result: notify.SuccessResult(notify.ChannelEmail, "email accepted")}
	slackSender := &fakeChannelSender{name: notify.ChannelSlack, result: notify.SuccessResult(notify.ChannelSlack, "slack accepted")}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.RunWithSenders(sendArgs(configPath, "--category", "backup", "--priority", "HIGH"), &stdout, &stderr, emailSender, slackSender)
	if code != diagnostics.ExitSuccess {
		t.Fatalf("RunWithSenders() exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if got := stdout.String(); got != "notification accepted\n" {
		t.Fatalf("stdout = %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if emailSender.calls != 1 || slackSender.calls != 1 {
		t.Fatalf("calls email=%d slack=%d, want 1 each", emailSender.calls, slackSender.calls)
	}
}

func TestRunWithSendersUsesCatchAll(t *testing.T) {
	configPath := writeIntegrationConfig(t)
	emailSender := &fakeChannelSender{name: notify.ChannelEmail, result: notify.SuccessResult(notify.ChannelEmail, "email accepted")}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.RunWithSenders(sendArgs(configPath), &stdout, &stderr, emailSender)
	if code != diagnostics.ExitSuccess {
		t.Fatalf("RunWithSenders() exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if emailSender.calls != 1 || emailSender.delivery.RouteID != "catch_all" {
		t.Fatalf("calls=%d delivery=%#v", emailSender.calls, emailSender.delivery)
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
			args:       []string{"send", "--sender", "BackupJob"},
			wantCode:   diagnostics.ExitInvalidInput,
			wantStderr: "invalid_input:",
		},
		{
			name:       "missing config",
			args:       sendArgs(filepath.Join(t.TempDir(), "missing.json")),
			wantCode:   diagnostics.ExitMissingConfig,
			wantStderr: "missing_config:",
		},
		{
			name:       "invalid config",
			args:       sendArgs(writeInvalidIntegrationConfig(t)),
			wantCode:   diagnostics.ExitInvalidConfig,
			wantStderr: "invalid_config:",
		},
		{
			name:       "attachment error",
			args:       append(sendArgs(configPath), "--attach", missingAttachment),
			senders:    []notify.ChannelSender{&fakeChannelSender{name: notify.ChannelEmail, result: notify.SuccessResult(notify.ChannelEmail, "accepted")}},
			wantCode:   diagnostics.ExitAttachmentError,
			wantStderr: "attachment_error:",
		},
		{
			name: "delivery failure",
			args: sendArgs(configPath),
			senders: []notify.ChannelSender{&fakeChannelSender{
				name: notify.ChannelEmail,
				err:  diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelEmail, "provider rejected request"),
			}},
			wantCode:   diagnostics.ExitDeliveryFailure,
			wantStderr: "delivery_failure: email:",
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
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRunWithSendersRedactsConfiguredSecretsFromDiagnostics(t *testing.T) {
	configPath := writeIntegrationConfig(t)
	sender := &fakeChannelSender{
		name: notify.ChannelEmail,
		err:  diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelEmail, "smtp password SMTP_PASSWORD_PLACEHOLDER rejected"),
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.RunWithSenders(sendArgs(configPath), &stdout, &stderr, sender)
	if code != diagnostics.ExitDeliveryFailure {
		t.Fatalf("RunWithSenders() exit code = %d, want delivery failure", code)
	}
	got := stderr.String()
	if strings.Contains(got, "SMTP_PASSWORD_PLACEHOLDER") {
		t.Fatalf("stderr leaked secret: %q", got)
	}
	if !strings.Contains(got, diagnostics.Redacted) {
		t.Fatalf("stderr = %q, want redacted marker", got)
	}
}

func TestRunWithSendersTelegramTopicsEndToEnd(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123456:ABCDEF/createForumTopic" && r.URL.Path != "/bot123456:ABCDEF/sendMessage" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Path == "/bot123456:ABCDEF/sendMessage" {
			if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/bot123456:ABCDEF/createForumTopic" {
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":77}}`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	configPath := writeTelegramTopicsConfig(t)
	topicStatePath := filepath.Join(filepath.Dir(configPath), "telegram-topics.json")
	sender := telegram.NewSender(server.Client(), telegram.WithBaseURL(server.URL), telegram.WithTopicStore(telegramtopics.NewFileRepository(topicStatePath)))
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.RunWithSenders(sendArgs(configPath, "--category", "backup"), &stdout, &stderr, sender)
	if code != diagnostics.ExitSuccess {
		t.Fatalf("RunWithSenders() exit code = %d, want success; stderr=%q", code, stderr.String())
	}
	if gotPayload["message_thread_id"] != float64(77) {
		t.Fatalf("message_thread_id = %#v, want 77", gotPayload["message_thread_id"])
	}
	if _, err := os.Stat(topicStatePath); err != nil {
		t.Fatalf("topic state was not written: %v", err)
	}
}

func TestRunWatchTelegramBOTRequiresAccountWhenMultipleTelegramAccountsExist(t *testing.T) {
	configPath := writeMultipleTelegramAccountsConfig(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.Run([]string{
		"watchTelegramBOT",
		"--config", configPath,
		"--max-seconds", "1",
		"--log", filepath.Join(t.TempDir(), "events.jsonl"),
	}, &stdout, &stderr)
	if code != diagnostics.ExitInvalidInput {
		t.Fatalf("Run() exit code = %d, want invalid input; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "multiple telegram delivery accounts") {
		t.Fatalf("stderr = %q, want multiple account diagnostic", stderr.String())
	}
}

func TestRunWatchTelegramBOTRejectsMissingTokenBeforePolling(t *testing.T) {
	configPath := writeTelegramAccountWithoutTokenConfig(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.Run([]string{
		"watchTelegramBOT",
		"--config", configPath,
		"--account", "telegram-main",
		"--max-seconds", "1",
		"--log", filepath.Join(t.TempDir(), "events.jsonl"),
	}, &stdout, &stderr)
	if code != diagnostics.ExitInvalidConfig {
		t.Fatalf("Run() exit code = %d, want invalid config; stderr=%q", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "123456:ABCDEF") {
		t.Fatalf("stderr leaked token: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "required secret \"token\" is missing") {
		t.Fatalf("stderr = %q, want missing token diagnostic", stderr.String())
	}
}

func sendArgs(configPath string, extra ...string) []string {
	args := []string{
		"send",
		"--config", configPath,
		"--sender", "TestRunner",
		"--title", "Test",
		"--message", "Integration test",
	}
	return append(args, extra...)
}

type fakeChannelSender struct {
	name     string
	result   notify.Result
	err      error
	calls    int
	request  notify.Request
	delivery notify.ResolvedDelivery
}

func (f *fakeChannelSender) Name() string {
	return f.name
}

func (f *fakeChannelSender) Send(_ context.Context, request notify.Request, delivery notify.ResolvedDelivery) (notify.Result, error) {
	f.calls++
	f.request = request
	f.delivery = delivery
	return f.result, f.err
}

func writeIntegrationConfig(t *testing.T) string {
	return writeFile(t, "noticli.json", `{
		"destinations": {
			"ops-email": {"type": "email", "email": "ops@example.invalid"},
			"ops-slack": {"type": "slack", "slack_destination": "#ops"}
		},
		"delivery_accounts": {
			"smtp-main": {
				"type": "email",
				"settings": {"host": "smtp.example.invalid", "port": "587", "from": "noticli@example.invalid"},
				"secrets": {"smtp_password": "SMTP_PASSWORD_PLACEHOLDER"},
				"attachments": "supported"
			},
			"slack-main": {
				"type": "slack",
				"settings": {"workspace": "ops"},
				"secrets": {"webhook_url": "https://hooks.slack.com/services/T/B/S"},
				"attachments": "unsupported"
			}
		},
		"routes": [
			{
				"id": "backup-high",
				"match": {"categories": ["backup"], "priorities": ["HIGH"]},
				"deliveries": [
					{"account": "smtp-main", "destination": "ops-email"},
					{"account": "slack-main", "destination": "ops-slack"}
				]
			}
		],
		"catch_all": {"deliveries": [{"account": "smtp-main", "destination": "ops-email"}]}
	}`)
}

func writeInvalidIntegrationConfig(t *testing.T) string {
	return writeFile(t, "invalid-noticli.json", `{
		"destinations": {"ops-email": {"type": "email", "email": "ops@example.invalid"}},
		"delivery_accounts": {}
	}`)
}

func writeTelegramTopicsConfig(t *testing.T) string {
	return writeFile(t, "telegram-noticli.json", `{
		"destinations": {
			"ops-telegram-topics": {
				"type": "telegram",
				"telegram_delivery_mode": "topics",
				"telegram_topic_group_chat_id": "-1001234567890",
				"telegram_topic_group_name": "Operations"
			}
		},
		"delivery_accounts": {
			"telegram-main": {
				"type": "telegram",
				"settings": {"parse_mode": "HTML"},
				"secrets": {"token": "123456:ABCDEF"},
				"attachments": "unsupported"
			}
		},
		"routes": [
			{
				"id": "backup-telegram",
				"match": {"categories": ["backup"]},
				"deliveries": [{"account": "telegram-main", "destination": "ops-telegram-topics"}]
			}
		],
		"catch_all": {"deliveries": [{"account": "telegram-main", "destination": "ops-telegram-topics"}]}
	}`)
}

func writeMultipleTelegramAccountsConfig(t *testing.T) string {
	return writeFile(t, "telegram-watch-multiple.json", `{
		"destinations": {
			"ops-telegram": {"type": "telegram", "telegram_chat_id": "12345"}
		},
		"delivery_accounts": {
			"telegram-main": {
				"type": "telegram",
				"settings": {},
				"secrets": {"token": "123456:ABCDEF"},
				"attachments": "unsupported"
			},
			"telegram-secondary": {
				"type": "telegram",
				"settings": {},
				"secrets": {"token": "654321:FEDCBA"},
				"attachments": "unsupported"
			}
		},
		"catch_all": {"deliveries": [{"account": "telegram-main", "destination": "ops-telegram"}]}
	}`)
}

func writeTelegramAccountWithoutTokenConfig(t *testing.T) string {
	return writeFile(t, "telegram-watch-missing-token.json", `{
		"destinations": {
			"ops-telegram": {"type": "telegram", "telegram_chat_id": "12345"}
		},
		"delivery_accounts": {
			"telegram-main": {
				"type": "telegram",
				"settings": {},
				"secrets": {},
				"attachments": "unsupported"
			}
		},
		"catch_all": {"deliveries": [{"account": "telegram-main", "destination": "ops-telegram"}]}
	}`)
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

type failingHTTPClient struct{}

func (failingHTTPClient) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected HTTP call")
}
