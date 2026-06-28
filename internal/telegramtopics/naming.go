package telegramtopics

import (
	"fmt"
	"hash/fnv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxTopicNameLength = 128

	defaultTopicName = "NotiCLI"
)

func AssociationKey(recipientID, chatID, sender string) string {
	return strings.TrimSpace(recipientID) + "\x00" + strings.TrimSpace(chatID) + "\x00" + strings.TrimSpace(sender)
}

func TopicNameForSender(recipientID, chatID, sender string, associations []Association) (string, string) {
	key := AssociationKey(recipientID, chatID, sender)
	for _, association := range associations {
		if association.Key() == key {
			return association.TopicName, association.TopicNameDisambiguator
		}
	}

	base := SanitizeTopicName(sender)
	if hasTopicNameCollision(recipientID, chatID, sender, base, associations) {
		disambiguator := topicNameDisambiguator(sender)
		return withDisambiguator(base, disambiguator), disambiguator
	}
	return base, ""
}

func SanitizeTopicName(sender string) string {
	var builder strings.Builder
	lastWasSpace := false
	for _, r := range strings.TrimSpace(sender) {
		if unicode.IsSpace(r) {
			if builder.Len() > 0 && !lastWasSpace {
				builder.WriteRune(' ')
				lastWasSpace = true
			}
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		builder.WriteRune(r)
		lastWasSpace = false
	}

	name := strings.TrimSpace(builder.String())
	if name == "" {
		name = defaultTopicName
	}
	return truncateRunes(name, MaxTopicNameLength)
}

func hasTopicNameCollision(recipientID, chatID, sender, topicName string, associations []Association) bool {
	for _, association := range associations {
		if strings.TrimSpace(association.RecipientID) != strings.TrimSpace(recipientID) {
			continue
		}
		if strings.TrimSpace(association.ChatID) != strings.TrimSpace(chatID) {
			continue
		}
		if strings.TrimSpace(association.Sender) == strings.TrimSpace(sender) {
			continue
		}
		if association.TopicName == topicName || SanitizeTopicName(association.Sender) == topicName {
			return true
		}
	}
	return false
}

func topicNameDisambiguator(sender string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.TrimSpace(sender)))
	return fmt.Sprintf("%08x", hash.Sum32())
}

func withDisambiguator(base, disambiguator string) string {
	suffix := " #" + disambiguator
	limit := MaxTopicNameLength - utf8.RuneCountInString(suffix)
	if limit < 1 {
		return truncateRunes(suffix, MaxTopicNameLength)
	}
	return truncateRunes(base, limit) + suffix
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	var builder strings.Builder
	count := 0
	for _, r := range value {
		if count == limit {
			break
		}
		builder.WriteRune(r)
		count++
	}
	return builder.String()
}
