package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rodrigogml/NotiCLI/internal/app"
	"github.com/rodrigogml/NotiCLI/internal/channels/email"
	"github.com/rodrigogml/NotiCLI/internal/channels/slack"
	"github.com/rodrigogml/NotiCLI/internal/channels/telegram"
	"github.com/rodrigogml/NotiCLI/internal/config"
	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
	"github.com/rodrigogml/NotiCLI/internal/telegramtopics"
)

const (
	CommandSend             = "send"
	CommandWatchTelegramBOT = "watchTelegramBOT"

	DefaultConfigFileName = "noticli.json"
	DefaultWatchLogPath   = "tmp/noticli.telegram-bot-events.jsonl"
	ExitInvalidInput      = diagnostics.ExitInvalidInput
)

type ParseError struct {
	Message  string
	ShowHelp bool
}

type WatchTelegramBOTRequest struct {
	ConfigPath  string
	AccountID   string
	PollSeconds int
	MaxSeconds  int
	LogPath     string
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
	flags.StringVar(&request.Category, "category", "", "notification category")
	flags.StringVar(&request.Priority, "priority", notify.PriorityNormal, "notification priority")
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
	request.Category = strings.TrimSpace(request.Category)
	request.Priority = strings.ToUpper(strings.TrimSpace(request.Priority))
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
	if request.Priority == "" {
		request.Priority = notify.PriorityNormal
	}
	if !notify.IsValidPriority(request.Priority) {
		return notify.Request{}, ParseError{Message: fmt.Sprintf("unsupported priority %q", request.Priority), ShowHelp: true}
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

func ParseWatchTelegramBOT(args []string) (WatchTelegramBOTRequest, error) {
	executablePath, _ := os.Executable()
	return ParseWatchTelegramBOTWithExecutablePath(args, executablePath)
}

func ParseWatchTelegramBOTWithExecutablePath(args []string, executablePath string) (WatchTelegramBOTRequest, error) {
	var request WatchTelegramBOTRequest

	flags := flag.NewFlagSet(CommandWatchTelegramBOT, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&request.ConfigPath, "config", "", "configuration file path")
	flags.StringVar(&request.AccountID, "account", "", "telegram delivery account id")
	flags.IntVar(&request.PollSeconds, "poll-seconds", 3, "telegram getUpdates long-poll timeout in seconds")
	flags.IntVar(&request.MaxSeconds, "max-seconds", 0, "maximum watch time in seconds; 0 runs until Ctrl+C")
	flags.StringVar(&request.LogPath, "log", DefaultWatchLogPath, "JSONL event log path")

	if err := flags.Parse(args); err != nil {
		return WatchTelegramBOTRequest{}, ParseError{Message: err.Error(), ShowHelp: true}
	}
	if flags.NArg() > 0 {
		return WatchTelegramBOTRequest{}, ParseError{Message: fmt.Sprintf("unexpected argument %q", flags.Arg(0)), ShowHelp: true}
	}

	configProvided := false
	accountProvided := false
	logProvided := false
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "config":
			configProvided = true
		case "account":
			accountProvided = true
		case "log":
			logProvided = true
		}
	})

	request.ConfigPath = strings.TrimSpace(request.ConfigPath)
	request.AccountID = strings.TrimSpace(request.AccountID)
	request.LogPath = strings.TrimSpace(request.LogPath)

	if configProvided && request.ConfigPath == "" {
		return WatchTelegramBOTRequest{}, ParseError{Message: "empty --config value", ShowHelp: true}
	}
	if request.ConfigPath == "" {
		request.ConfigPath = DefaultConfigPath(executablePath)
	}
	if accountProvided && request.AccountID == "" {
		return WatchTelegramBOTRequest{}, ParseError{Message: "empty --account value", ShowHelp: true}
	}
	if request.PollSeconds <= 0 {
		return WatchTelegramBOTRequest{}, ParseError{Message: "--poll-seconds must be greater than 0", ShowHelp: true}
	}
	if request.MaxSeconds < 0 {
		return WatchTelegramBOTRequest{}, ParseError{Message: "--max-seconds must be 0 or greater", ShowHelp: true}
	}
	if logProvided && request.LogPath == "" {
		return WatchTelegramBOTRequest{}, ParseError{Message: "empty --log value", ShowHelp: true}
	}
	if request.LogPath == "" {
		request.LogPath = DefaultWatchLogPath
	}

	return request, nil
}

func DefaultConfigPath(executablePath string) string {
	resolved := resolvedExecutablePath(executablePath)
	if resolved == "" {
		return filepath.Join("config", DefaultConfigFileName)
	}
	return filepath.Join(filepath.Dir(resolved), "config", DefaultConfigFileName)
}

func resolvedExecutablePath(executablePath string) string {
	chain := executablePathChain(executablePath)
	if len(chain) == 0 {
		return ""
	}
	return chain[len(chain)-1]
}

func executablePathChain(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	chain := make([]string, 0, 4)
	seen := make(map[string]struct{})
	current := path
	for current != "" {
		if _, ok := seen[current]; ok {
			break
		}
		seen[current] = struct{}{}
		chain = append(chain, current)

		target, err := os.Readlink(current)
		if err != nil {
			break
		}
		if filepath.IsAbs(target) {
			current = target
			continue
		}
		current = filepath.Join(filepath.Dir(current), target)
	}
	return chain
}

func isSupportedChannel(channel string) bool {
	return notify.IsSupportedChannel(channel)
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == CommandWatchTelegramBOT {
		return runWatchTelegramBOT(args[1:], stdout, stderr)
	}

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

	result, err := app.New(configuration, defaultSenders(request.ConfigPath)...).Notify(context.Background(), request)
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

func runWatchTelegramBOT(args []string, stdout, stderr io.Writer) int {
	request, err := ParseWatchTelegramBOT(args)
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

	accountID, account, err := selectTelegramAccount(configuration, request.AccountID)
	redactor := diagnostics.NewRedactor(configuration.SecretValues()...)
	if err != nil {
		return diagnostics.WriteFailureWithRedactor(stderr, err, redactor)
	}
	token := strings.TrimSpace(account.Secrets["token"])
	if token == "" {
		return diagnostics.WriteFailureWithRedactor(stderr, diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, notify.ChannelTelegram, "required secret \"token\" is missing"), redactor)
	}

	if err := os.MkdirAll(filepath.Dir(request.LogPath), 0o700); err != nil {
		return diagnostics.WriteFailureWithRedactor(stderr, diagnostics.New(diagnostics.CategoryInternalError, fmt.Sprintf("watch log directory could not be created: %s", err.Error())), redactor)
	}
	logFile, err := os.OpenFile(request.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return diagnostics.WriteFailureWithRedactor(stderr, diagnostics.New(diagnostics.CategoryInternalError, fmt.Sprintf("watch log could not be opened: %s", err.Error())), redactor)
	}
	defer logFile.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	watcher := telegram.NewWatcher(nil)
	err = watcher.Watch(ctx, telegram.WatchOptions{
		Token:       token,
		AccountID:   accountID,
		PollTimeout: time.Duration(request.PollSeconds) * time.Second,
		MaxDuration: time.Duration(request.MaxSeconds) * time.Second,
		OnStart: func(start telegram.WatchStart) {
			fmt.Fprintf(stdout, "Watching Telegram bot account %q with getUpdates timeout %s.\n", start.AccountID, start.PollTimeout)
			fmt.Fprintf(stdout, "Writing raw events to %s.\n", request.LogPath)
			if start.MaxDuration > 0 {
				fmt.Fprintf(stdout, "The watcher will stop automatically after %s.\n", start.MaxDuration)
			} else {
				fmt.Fprintln(stdout, "Press Ctrl+C to stop.")
			}
		},
		OnUpdate: func(update telegram.WatchUpdate) error {
			if err := writeWatchEvent(stdout, update); err != nil {
				return err
			}
			return appendWatchLog(logFile, update)
		},
	})
	if err != nil {
		return diagnostics.WriteFailureWithRedactor(stderr, diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelTelegram, err.Error()), redactor)
	}
	return diagnostics.ExitSuccess
}

func defaultSenders(configPath string) []notify.ChannelSender {
	topicStore := telegramtopics.NewFileRepository(telegramtopics.StatePathForConfig(configPath))
	return []notify.ChannelSender{
		email.Sender{},
		telegram.NewSenderWithTopicStore(nil, topicStore),
		slack.Sender{},
	}
}

func RunWithSenders(args []string, stdout, stderr io.Writer, senders ...notify.ChannelSender) int {
	if len(args) > 0 && args[0] == CommandWatchTelegramBOT {
		return runWatchTelegramBOT(args[1:], stdout, stderr)
	}

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

func selectTelegramAccount(configuration notify.Configuration, accountID string) (string, notify.DeliveryAccount, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID != "" {
		account, ok := configuration.DeliveryAccounts[accountID]
		if !ok {
			return "", notify.DeliveryAccount{}, diagnostics.ForChannel(diagnostics.CategoryMissingConfig, notify.ChannelTelegram, fmt.Sprintf("telegram delivery account %q is not configured", accountID))
		}
		if account.Type != notify.ChannelTelegram {
			return "", notify.DeliveryAccount{}, diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, notify.ChannelTelegram, fmt.Sprintf("delivery account %q is type %q, not telegram", accountID, account.Type))
		}
		if !account.Enabled {
			return "", notify.DeliveryAccount{}, diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, notify.ChannelTelegram, fmt.Sprintf("delivery account %q is disabled", accountID))
		}
		return accountID, account, nil
	}

	var selectedID string
	var selected notify.DeliveryAccount
	count := 0
	for id, account := range configuration.DeliveryAccounts {
		if account.Type != notify.ChannelTelegram || !account.Enabled {
			continue
		}
		count++
		selectedID = id
		selected = account
	}
	if count == 0 {
		return "", notify.DeliveryAccount{}, diagnostics.ForChannel(diagnostics.CategoryMissingConfig, notify.ChannelTelegram, "no enabled telegram delivery account is configured")
	}
	if count > 1 {
		return "", notify.DeliveryAccount{}, diagnostics.New(diagnostics.CategoryInvalidInput, "multiple telegram delivery accounts are configured; pass --account <id>")
	}
	return selectedID, selected, nil
}

func writeWatchEvent(w io.Writer, update telegram.WatchUpdate) error {
	summary := update.Summary
	fmt.Fprintf(w, "telegram_update update_id=%d type=%s observed_at=%s\n", update.UpdateID, update.UpdateType, update.ObservedAt.Format(time.RFC3339Nano))
	if summary.ChatID != "" || summary.ChatType != "" || summary.ChatTitle != "" || summary.MessageThreadID > 0 {
		fmt.Fprintf(w, "  chat id=%s type=%s title=%q thread_id=%d\n", summary.ChatID, summary.ChatType, summary.ChatTitle, summary.MessageThreadID)
	}
	if summary.FromID != "" || summary.FromUsername != "" || summary.FromFirstName != "" {
		fmt.Fprintf(w, "  from id=%s username=%q first_name=%q\n", summary.FromID, summary.FromUsername, summary.FromFirstName)
	}
	if summary.Text != "" {
		fmt.Fprintf(w, "  text=%q\n", summary.Text)
	}
	if summary.Caption != "" {
		fmt.Fprintf(w, "  caption=%q\n", summary.Caption)
	}
	return nil
}

func appendWatchLog(w io.Writer, update telegram.WatchUpdate) error {
	record := map[string]any{
		"observed_at": update.ObservedAt.Format(time.RFC3339Nano),
		"account_id":  update.AccountID,
		"update_id":   update.UpdateID,
		"update_type": update.UpdateType,
		"summary":     update.Summary,
	}
	var raw any
	if err := json.Unmarshal(update.Raw, &raw); err == nil {
		record["update"] = raw
	} else {
		record["update"] = string(update.Raw)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func writeUsageFailure(w io.Writer, message string) int {
	fmt.Fprintf(w, "%s: %s\n\n%s", diagnostics.CategoryInvalidInput, strings.TrimSpace(message), Usage())
	return diagnostics.ExitInvalidInput
}

func Usage() string {
	return strings.TrimLeft(`
Usage:
  noticli send --sender <system> --title <text> --message <text> [--priority <HIGH|NORMAL|LOW>] [--category <text>] [--config <path>] [--attach <path>...]
  noticli watchTelegramBOT [--config <path>] [--account <id>] [--poll-seconds <n>] [--max-seconds <n>] [--log <path>]

Required flags:
  --sender     Calling system identifier, up to 20 characters.
  --title      Notification title or subject.
  --message    Notification body.

Optional flags:
  --config     JSON configuration file. Defaults to config/noticli.json beside the resolved executable path.
  --category   Routing category matched by noticli.json routes.
  --priority   Routing priority: HIGH, NORMAL or LOW. Defaults to NORMAL.
  --attach     Readable file attachment. May be repeated; unsupported destinations receive the message without attachments.
  --account    Telegram delivery account ID for watchTelegramBOT. Required only when multiple Telegram accounts are configured.
  --poll-seconds  Telegram getUpdates long-poll timeout. Defaults to 3.
  --max-seconds   Maximum watch duration. Defaults to 0, which runs until Ctrl+C.
  --log        JSONL event log path for watchTelegramBOT. Defaults to tmp/noticli.telegram-bot-events.jsonl.

Examples:
  noticli send --sender BackupJob --category backup --priority HIGH --title "Backup failed" --message "Nightly backup failed on server-01"
  noticli send --config /opt/NotiCLI/current/config/noticli.json --sender DeployBot --title "Deploy complete" --message "Release completed"
  noticli watchTelegramBOT --config ./noticli.json --account telegram-main --max-seconds 60
`, "\n")
}
