package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

const (
	settingFrom     = "from"
	settingFromName = "from_name"
	settingHost     = "host"
	settingPort     = "port"
	settingUsername = "username"

	secretSMTPPassword = "smtp_password"
)

type Transport interface {
	Send(ctx context.Context, message Message) error
}

type Message struct {
	Host         string
	Port         string
	Username     string
	Password     string
	From         string
	FromName     string
	To           string
	Subject      string
	Body         string
	SenderSystem string
	Attachments  []notify.Attachment
}

type Sender struct {
	transport Transport
}

func NewSender(transport Transport) Sender {
	return Sender{transport: transport}
}

func (Sender) Name() string {
	return notify.ChannelEmail
}

func (s Sender) Send(ctx context.Context, request notify.Request, delivery notify.ResolvedDelivery) (notify.Result, error) {
	message, err := buildMessage(request, delivery.Destination, delivery.Account)
	if err != nil {
		return notify.FailureResult(diagnostics.CategoryInvalidConfig, notify.ChannelEmail, diagnostics.FromError(err).Message), err
	}

	transport := s.transport
	if transport == nil {
		transport = SMTPTransport{}
	}
	if err := transport.Send(ctx, message); err != nil {
		diagnostic := diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelEmail, fmt.Sprintf("provider rejected email: %s", err))
		return notify.FailureResult(diagnostic.Category, diagnostic.Channel, diagnostic.Message), diagnostic
	}

	return notify.SuccessResult(notify.ChannelEmail, "email accepted"), nil
}

func buildMessage(request notify.Request, destination notify.Destination, config notify.DeliveryAccount) (Message, error) {
	if config.Type != notify.ChannelEmail {
		return Message{}, invalidConfig("channel config type must be email")
	}

	from, err := requiredSetting(config, settingFrom)
	if err != nil {
		return Message{}, err
	}
	host, err := requiredSetting(config, settingHost)
	if err != nil {
		return Message{}, err
	}
	port, err := requiredSetting(config, settingPort)
	if err != nil {
		return Message{}, err
	}
	password, err := requiredSecret(config, secretSMTPPassword)
	if err != nil {
		return Message{}, err
	}
	to, ok := destination.Address()
	if !ok {
		return Message{}, invalidConfig("destination has no email address")
	}

	username := strings.TrimSpace(config.Settings[settingUsername])
	if username == "" {
		username = from
	}

	return Message{
		Host:         host,
		Port:         port,
		Username:     username,
		Password:     password,
		From:         from,
		FromName:     strings.TrimSpace(config.Settings[settingFromName]),
		To:           to,
		Subject:      formatSubject(request),
		Body:         request.Message,
		SenderSystem: request.SenderSystem,
		Attachments:  request.Attachments,
	}, nil
}

func requiredSetting(config notify.DeliveryAccount, key string) (string, error) {
	value := strings.TrimSpace(config.Settings[key])
	if value == "" {
		return "", invalidConfig(fmt.Sprintf("required setting %q is missing", key))
	}
	return value, nil
}

func requiredSecret(config notify.DeliveryAccount, key string) (string, error) {
	value := strings.TrimSpace(config.Secrets[key])
	if value == "" {
		return "", invalidConfig(fmt.Sprintf("required secret %q is missing", key))
	}
	return value, nil
}

func invalidConfig(message string) error {
	return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, notify.ChannelEmail, message)
}

type SMTPTransport struct{}

func (SMTPTransport) Send(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	address := net.JoinHostPort(message.Host, message.Port)
	auth := smtp.PlainAuth("", message.Username, message.Password, message.Host)
	data, err := formatSMTPMessage(message)
	if err != nil {
		return err
	}
	return smtp.SendMail(address, auth, message.From, []string{message.To}, data)
}

func formatPlainTextMessage(message Message) string {
	headers := []string{
		fmt.Sprintf("From: %s", formatAddress(message.FromName, message.From)),
		fmt.Sprintf("To: %s", message.To),
		fmt.Sprintf("Subject: %s", message.Subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
	}
	if strings.TrimSpace(message.SenderSystem) != "" {
		headers = append(headers, fmt.Sprintf("X-NotiCLI-Sender: %s", message.SenderSystem))
	}

	return strings.Join(headers, "\r\n") + "\r\n\r\n" + message.Body + "\r\n"
}

func formatSMTPMessage(message Message) ([]byte, error) {
	if len(message.Attachments) == 0 {
		return []byte(formatPlainTextMessage(message)), nil
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	headers := []string{
		fmt.Sprintf("From: %s", formatAddress(message.FromName, message.From)),
		fmt.Sprintf("To: %s", message.To),
		fmt.Sprintf("Subject: %s", message.Subject),
		"MIME-Version: 1.0",
		fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s", writer.Boundary()),
	}
	if strings.TrimSpace(message.SenderSystem) != "" {
		headers = append(headers, fmt.Sprintf("X-NotiCLI-Sender: %s", message.SenderSystem))
	}
	buffer.WriteString(strings.Join(headers, "\r\n"))
	buffer.WriteString("\r\n\r\n")

	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	textPart, err := writer.CreatePart(textHeader)
	if err != nil {
		return nil, err
	}
	if _, err := textPart.Write([]byte(message.Body)); err != nil {
		return nil, err
	}

	for _, attachment := range message.Attachments {
		if err := writeAttachmentPart(writer, attachment); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func formatAddress(name, address string) string {
	address = strings.TrimSpace(address)
	name = strings.TrimSpace(name)
	if name == "" {
		return address
	}
	return (&mail.Address{Name: name, Address: address}).String()
}

func formatSubject(request notify.Request) string {
	senderSystem := sanitizeHeaderValue(strings.TrimSpace(request.SenderSystem))
	priority := sanitizeHeaderValue(request.EffectivePriority())
	title := sanitizeHeaderValue(strings.TrimSpace(request.Title))
	if senderSystem == "" {
		return title
	}
	return fmt.Sprintf("[%s] [%s] %s", senderSystem, priority, title)
}

func sanitizeHeaderValue(value string) string {
	replacer := strings.NewReplacer("\r", " ", "\n", " ")
	return strings.Join(strings.Fields(replacer.Replace(value)), " ")
}

func writeAttachmentPart(writer *multipart.Writer, attachment notify.Attachment) error {
	data, err := os.ReadFile(attachment.Path)
	if err != nil {
		return err
	}

	header := textproto.MIMEHeader{}
	contentType := strings.TrimSpace(attachment.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header.Set("Content-Type", contentType)
	header.Set("Content-Transfer-Encoding", "base64")
	header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, attachment.EffectiveFilename()))

	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}

	_, err = part.Write([]byte(base64.StdEncoding.EncodeToString(data)))
	return err
}
