package diagnostics

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type Category string

const (
	CategorySuccess         Category = "success"
	CategoryInvalidInput    Category = "invalid_input"
	CategoryMissingConfig   Category = "missing_config"
	CategoryInvalidConfig   Category = "invalid_config"
	CategoryAttachmentError Category = "attachment_error"
	CategoryDeliveryFailure Category = "delivery_failure"
	CategoryInternalError   Category = "internal_error"
)

const (
	ExitSuccess         = 0
	ExitInternalError   = 1
	ExitInvalidInput    = 2
	ExitMissingConfig   = 3
	ExitInvalidConfig   = 4
	ExitAttachmentError = 5
	ExitDeliveryFailure = 6
)

type Diagnostic struct {
	Category Category
	Channel  string
	Message  string
}

func New(category Category, message string) Diagnostic {
	return Diagnostic{
		Category: category,
		Message:  strings.TrimSpace(message),
	}
}

func ForChannel(category Category, channel, message string) Diagnostic {
	diagnostic := New(category, message)
	diagnostic.Channel = strings.TrimSpace(channel)
	return diagnostic
}

func (d Diagnostic) Error() string {
	if d.Message == "" {
		return string(d.Category)
	}
	if d.Channel != "" {
		return fmt.Sprintf("%s: %s: %s", d.Category, d.Channel, d.Message)
	}
	return fmt.Sprintf("%s: %s", d.Category, d.Message)
}

func FromError(err error) Diagnostic {
	if err == nil {
		return New(CategorySuccess, "")
	}

	var diagnostic Diagnostic
	if errors.As(err, &diagnostic) {
		return diagnostic
	}

	return New(CategoryInternalError, err.Error())
}

func ExitCode(category Category) int {
	switch category {
	case CategorySuccess:
		return ExitSuccess
	case CategoryInvalidInput:
		return ExitInvalidInput
	case CategoryMissingConfig:
		return ExitMissingConfig
	case CategoryInvalidConfig:
		return ExitInvalidConfig
	case CategoryAttachmentError:
		return ExitAttachmentError
	case CategoryDeliveryFailure:
		return ExitDeliveryFailure
	default:
		return ExitInternalError
	}
}

func WriteFailure(w io.Writer, err error) int {
	diagnostic := FromError(err)
	if diagnostic.Category == CategorySuccess {
		return ExitSuccess
	}
	fmt.Fprintln(w, diagnostic.Error())
	return ExitCode(diagnostic.Category)
}
