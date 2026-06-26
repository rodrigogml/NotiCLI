package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

const (
	CommandSend = "send"

	DefaultConfigFileName = "noticli.json"
	ExitInvalidInput      = diagnostics.ExitInvalidInput
)

type ParseError struct {
	Message string
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
		return notify.Request{}, ParseError{Message: "missing command"}
	}

	switch args[0] {
	case CommandSend:
		return parseSend(args[1:], executablePath)
	default:
		return notify.Request{}, ParseError{Message: fmt.Sprintf("unknown command %q", args[0])}
	}
}

func parseSend(args []string, executablePath string) (notify.Request, error) {
	var attachments attachmentFlags
	var request notify.Request

	flags := flag.NewFlagSet(CommandSend, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&request.ConfigPath, "config", "", "configuration file path")
	flags.StringVar(&request.RecipientID, "recipient", "", "configured recipient identifier")
	flags.StringVar(&request.Channel, "channel", "", "notification channel")
	flags.StringVar(&request.Title, "title", "", "notification title")
	flags.StringVar(&request.Message, "message", "", "notification message")
	flags.Var(&attachments, "attach", "attachment path")

	if err := flags.Parse(args); err != nil {
		return notify.Request{}, ParseError{Message: err.Error()}
	}
	if flags.NArg() > 0 {
		return notify.Request{}, ParseError{Message: fmt.Sprintf("unexpected argument %q", flags.Arg(0))}
	}

	configProvided := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			configProvided = true
		}
	})

	request.ConfigPath = strings.TrimSpace(request.ConfigPath)
	request.RecipientID = strings.TrimSpace(request.RecipientID)
	request.Channel = strings.TrimSpace(request.Channel)
	request.Title = strings.TrimSpace(request.Title)
	request.Message = strings.TrimSpace(request.Message)

	if configProvided && request.ConfigPath == "" {
		return notify.Request{}, ParseError{Message: "empty --config value"}
	}
	if request.ConfigPath == "" {
		request.ConfigPath = DefaultConfigPath(executablePath)
	}
	if request.RecipientID == "" {
		return notify.Request{}, ParseError{Message: "missing required flag --recipient"}
	}
	if request.Channel == "" {
		return notify.Request{}, ParseError{Message: "missing required flag --channel"}
	}
	if !isSupportedChannel(request.Channel) {
		return notify.Request{}, ParseError{Message: fmt.Sprintf("unsupported channel %q", request.Channel)}
	}
	if request.Title == "" {
		return notify.Request{}, ParseError{Message: "missing required flag --title"}
	}
	if request.Message == "" {
		return notify.Request{}, ParseError{Message: "missing required flag --message"}
	}

	for _, path := range attachments {
		path = strings.TrimSpace(path)
		if path == "" {
			return notify.Request{}, ParseError{Message: "empty --attach value"}
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
	switch channel {
	case notify.ChannelEmail, notify.ChannelSlack, notify.ChannelTelegram:
		return true
	default:
		return false
	}
}

func Run(args []string, stdout, stderr io.Writer) int {
	if _, err := Parse(args); err != nil {
		return diagnostics.WriteFailure(stderr, diagnostics.New(diagnostics.CategoryInvalidInput, err.Error()))
	}

	return diagnostics.WriteFailure(stderr, diagnostics.New(diagnostics.CategoryInternalError, "dispatch not implemented"))
}
