package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rodrigogml/NotiCLI/internal/app"
	"github.com/rodrigogml/NotiCLI/internal/channels/email"
	"github.com/rodrigogml/NotiCLI/internal/channels/slack"
	"github.com/rodrigogml/NotiCLI/internal/channels/telegram"
	"github.com/rodrigogml/NotiCLI/internal/config"
	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

const (
	CommandSend = "send"

	DefaultConfigFileName = "noticli.json"
	ExitInvalidInput      = diagnostics.ExitInvalidInput
)

type ParseError struct {
	Message  string
	ShowHelp bool
}

func (e ParseError) Error() string {
	return e.Message
}

func Parse(args []string) (notify.Request, error) {
	executablePath, _ := os.Executable()
	return ParseWithExecutablePath(args, executablePath)
}

func ParseWithExecutablePath(args []string, executablePath string) (notify.Request, error) {
	if len(args) == 0 {
		return notify.Request{}, ParseError{Message: "missing command", ShowHelp: true}
	}

	switch args[0] {
	case CommandSend:
		return parseSend(args[1:], executablePath)
	default:
		return notify.Request{}, ParseError{Message: fmt.Sprintf("unknown command %q", args[0]), ShowHelp: true}
	}
}

func parseSend(args []string, executablePath string) (notify.Request, error) {
	var attachments attachmentFlags
	var request notify.Request

	flags := flag.NewFlagSet(CommandSend, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&request.ConfigPath, "config", "", "configuration file path")
	flags.StringVar(&request.SenderSystem, "sender", "", "sending system identifier")
	flags.StringVar(&request.RecipientID, "recipient", "", "configured recipient identifier")
	flags.StringVar(&request.Channel, "channel", "", "notification channel")
	flags.StringVar(&request.Title, "title", "", "notification title")
	flags.StringVar(&request.Message, "message", "", "notification message")
	flags.Var(&attachments, "attach", "attachment path")

	if err := flags.Parse(args); err != nil {
		return notify.Request{}, ParseError{Message: err.Error(), ShowHelp: true}
	}
	if flags.NArg() > 0 {
		return notify.Request{}, ParseError{Message: fmt.Sprintf("unexpected argument %q", flags.Arg(0)), ShowHelp: true}
	}

	configProvided := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			configProvided = true
		}
	})

	request.ConfigPath = strings.TrimSpace(request.ConfigPath)
	request.SenderSystem = strings.TrimSpace(request.SenderSystem)
	request.RecipientID = strings.TrimSpace(request.RecipientID)
	request.Channel = strings.TrimSpace(request.Channel)
	request.Title = strings.TrimSpace(request.Title)
	request.Message = strings.TrimSpace(request.Message)

	if configProvided && request.ConfigPath == "" {
		return notify.Request{}, ParseError{Message: "empty --config value", ShowHelp: true}
	}
	if request.ConfigPath == "" {
		request.ConfigPath = DefaultConfigPath(executablePath)
	}
	if request.SenderSystem == "" {
		return notify.Request{}, ParseError{Message: "missing required flag --sender", ShowHelp: true}
	}
	if len([]rune(request.SenderSystem)) > notify.MaxSenderSystemLength {
		return notify.Request{}, ParseError{Message: fmt.Sprintf("--sender must be at most %d characters", notify.MaxSenderSystemLength), ShowHelp: true}
	}
	if request.RecipientID == "" {
		return notify.Request{}, ParseError{Message: "missing required flag --recipient", ShowHelp: true}
	}
	if request.Channel == "" {
		return notify.Request{}, ParseError{Message: "missing required flag --channel", ShowHelp: true}
	}
	if !isSupportedChannel(request.Channel) {
		return notify.Request{}, ParseError{Message: fmt.Sprintf("unsupported channel %q", request.Channel), ShowHelp: true}
	}
	if request.Title == "" {
		return notify.Request{}, ParseError{Message: "missing required flag --title", ShowHelp: true}
	}
	if request.Message == "" {
		return notify.Request{}, ParseError{Message: "missing required flag --message", ShowHelp: true}
	}

	for _, path := range attachments {
		path = strings.TrimSpace(path)
		if path == "" {
			return notify.Request{}, ParseError{Message: "empty --attach value", ShowHelp: true}
		}
		request.Attachments = append(request.Attachments, notify.Attachment{Path: path})
	}

	return request, nil
}

func DefaultConfigPath(executablePath string) string {
	if executablePath == "" {
		return DefaultConfigFileName
	}
	return filepath.Join(filepath.Dir(executablePath), DefaultConfigFileName)
}

func isSupportedChannel(channel string) bool {
	return notify.IsSupportedChannel(channel)
}

func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithSenders(args, stdout, stderr, email.Sender{}, telegram.Sender{}, slack.Sender{})
}

func RunWithSenders(args []string, stdout, stderr io.Writer, senders ...notify.ChannelSender) int {
	request, err := Parse(args)
	if err != nil {
		var parseErr ParseError
		if errors.As(err, &parseErr) && parseErr.ShowHelp {
			return writeUsageFailure(stderr, parseErr.Message)
		}
		return diagnostics.WriteFailure(stderr, diagnostics.New(diagnostics.CategoryInvalidInput, err.Error()))
	}
	configuration, err := config.Load(request.ConfigPath)
	if err != nil {
		return diagnostics.WriteFailure(stderr, err)
	}

	result, err := app.New(configuration, senders...).Notify(context.Background(), request)
	redactor := diagnostics.NewRedactor(configuration.SecretValues()...)
	if err != nil {
		return diagnostics.WriteFailureWithRedactor(stderr, err, redactor)
	}
	if !result.Success {
		return diagnostics.WriteFailureWithRedactor(stderr, diagnostics.ForChannel(result.Category, result.Channel, result.Message), redactor)
	}
	if strings.TrimSpace(result.Message) != "" {
		fmt.Fprintln(stdout, strings.TrimSpace(result.Message))
	}

	return result.ExitCode
}

func writeUsageFailure(w io.Writer, message string) int {
	fmt.Fprintf(w, "%s: %s\n\n%s", diagnostics.CategoryInvalidInput, strings.TrimSpace(message), Usage())
	return diagnostics.ExitInvalidInput
}

func Usage() string {
	return strings.TrimLeft(`
Usage:
  noticli send --sender <system> --recipient <id> --channel <email|telegram|slack> --title <text> --message <text> [--config <path>] [--attach <path>...]

Required flags:
  --sender     Calling system identifier, up to 20 characters.
  --recipient  Recipient ID configured in noticli.json.
  --channel    Delivery channel: email, telegram or slack.
  --title      Notification title or subject.
  --message    Notification body.

Optional flags:
  --config     JSON configuration file. Defaults to noticli.json beside the executable.
  --attach     Readable file attachment. May be repeated. Supported by email only.

Examples:
  noticli send --sender BackupJob --recipient ops --channel email --title "Backup failed" --message "Nightly backup failed on server-01"
  noticli send --config /opt/NotiCLI/config/noticli.json --sender DeployBot --recipient ops --channel slack --title "Deploy complete" --message "Release completed"
`, "\n")
}
