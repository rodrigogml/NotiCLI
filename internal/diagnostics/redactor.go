package diagnostics

import (
	"regexp"
	"strings"
)

const Redacted = "[REDACTED]"

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(token|password|passwd|secret|webhook|webhook_url|authorization)(\s*[:=]\s*)([^\s,;]+)`),
	regexp.MustCompile(`(?i)(bearer\s+)([A-Za-z0-9._~+/=-]+)`),
	regexp.MustCompile(`https://hooks\.slack\.com/services/[^\s]+`),
	regexp.MustCompile(`\b[0-9]{6,}:[A-Za-z0-9_-]{20,}\b`),
}

type Redactor struct {
	secrets []string
}

func NewRedactor(secrets ...string) Redactor {
	redactor := Redactor{}
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret != "" {
			redactor.secrets = append(redactor.secrets, secret)
		}
	}
	return redactor
}

func (r Redactor) Redact(message string) string {
	message = singleLine(message)
	for _, secret := range r.secrets {
		message = strings.ReplaceAll(message, secret, Redacted)
	}
	for _, pattern := range sensitivePatterns {
		message = pattern.ReplaceAllStringFunc(message, redactPatternMatch)
	}
	return message
}

func singleLine(message string) string {
	return strings.Join(strings.Fields(message), " ")
}

func redactPatternMatch(match string) string {
	for _, prefixPattern := range []string{":", "="} {
		if index := strings.Index(match, prefixPattern); index >= 0 {
			return strings.TrimSpace(match[:index+1]) + " " + Redacted
		}
	}
	fields := strings.Fields(match)
	if len(fields) > 1 && strings.EqualFold(fields[0], "bearer") {
		return fields[0] + " " + Redacted
	}
	return Redacted
}
