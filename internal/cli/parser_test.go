package cli_test

import (
	"bytes"
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/cli"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

func TestParseSendMapsFlagsToNotificationRequest(t *testing.T) {
	request, err := cli.Parse([]string{
		"send",
		"--config", "./noticli.json",
		"--recipient", "ops",
		"--channel", "email",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
		"--attach", "./a.txt",
		"--attach", "./b.txt",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if request.ConfigPath != "./noticli.json" {
		t.Fatalf("ConfigPath = %q", request.ConfigPath)
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

func TestParseSendRejectsMissingRequiredFlags(t *testing.T) {
	_, err := cli.Parse([]string{
		"send",
		"--channel", "email",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	})
	if err == nil {
		t.Fatal("Parse() error = nil, want missing recipient error")
	}
}

func TestParseSendRejectsUnsupportedChannel(t *testing.T) {
	_, err := cli.Parse([]string{
		"send",
		"--recipient", "ops",
		"--channel", "sms",
		"--title", "Backup failed",
		"--message", "Nightly backup failed",
	})
	if err == nil {
		t.Fatal("Parse() error = nil, want unsupported channel error")
	}
}

func TestRunIsNonInteractiveAndReturnsInvalidInputForParseErrors(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := cli.Run([]string{"send", "--recipient", "ops"}, &stdout, &stderr)
	if code != cli.ExitInvalidInput {
		t.Fatalf("Run() exit code = %d, want %d", code, cli.ExitInvalidInput)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() == "" {
		t.Fatal("stderr is empty, want diagnostic")
	}
}
