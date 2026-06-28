package telegramtopics_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rodrigogml/NotiCLI/internal/telegramtopics"
)

func TestAssociationKeyUsesRecipientChatAndSender(t *testing.T) {
	first := telegramtopics.AssociationKey("ops", "-100", "BackupJob")
	second := telegramtopics.AssociationKey("ops", "-100", "BillingJob")
	otherChat := telegramtopics.AssociationKey("ops", "-200", "BackupJob")

	if first == second {
		t.Fatal("same recipient/chat with different sender produced same key")
	}
	if first == otherChat {
		t.Fatal("same recipient/sender with different chat produced same key")
	}
}

func TestSanitizeTopicNameNormalizesSender(t *testing.T) {
	got := telegramtopics.SanitizeTopicName("  Backup\t\tJob\nProd\u0000  ")
	if got != "Backup Job Prod" {
		t.Fatalf("SanitizeTopicName() = %q, want normalized name", got)
	}
}

func TestSanitizeTopicNameFallsBackWhenSenderHasOnlyUnsafeCharacters(t *testing.T) {
	got := telegramtopics.SanitizeTopicName("\n\t\u0000")
	if got != "NotiCLI" {
		t.Fatalf("SanitizeTopicName() = %q, want fallback", got)
	}
}

func TestSanitizeTopicNameRespectsTelegramLengthLimit(t *testing.T) {
	got := telegramtopics.SanitizeTopicName(strings.Repeat("a", telegramtopics.MaxTopicNameLength+10))
	if utf8.RuneCountInString(got) != telegramtopics.MaxTopicNameLength {
		t.Fatalf("topic name length = %d, want %d", utf8.RuneCountInString(got), telegramtopics.MaxTopicNameLength)
	}
}

func TestTopicNameForSenderReturnsExistingAssociationName(t *testing.T) {
	name, disambiguator := telegramtopics.TopicNameForSender("ops", "-100", "BackupJob", []telegramtopics.Association{
		{
			RecipientID:            "ops",
			ChatID:                 "-100",
			Sender:                 "BackupJob",
			TopicName:              "Backup Job",
			TopicNameDisambiguator: "existing",
			MessageThreadID:        4,
			CreatedByNotiCLI:       true,
			Status:                 telegramtopics.TopicStatusActive,
		},
	})
	if name != "Backup Job" || disambiguator != "existing" {
		t.Fatalf("TopicNameForSender() = %q, %q; want existing association", name, disambiguator)
	}
}

func TestTopicNameForSenderDisambiguatesCollisionsDeterministically(t *testing.T) {
	associations := []telegramtopics.Association{
		{
			RecipientID:      "ops",
			ChatID:           "-100",
			Sender:           "Backup\tJob",
			TopicName:        "Backup Job",
			MessageThreadID:  4,
			CreatedByNotiCLI: true,
			Status:           telegramtopics.TopicStatusActive,
		},
	}

	firstName, firstDisambiguator := telegramtopics.TopicNameForSender("ops", "-100", "Backup  Job", associations)
	secondName, secondDisambiguator := telegramtopics.TopicNameForSender("ops", "-100", "Backup  Job", associations)

	if firstDisambiguator == "" {
		t.Fatal("disambiguator is empty, want deterministic suffix")
	}
	if firstName != secondName || firstDisambiguator != secondDisambiguator {
		t.Fatalf("TopicNameForSender() not deterministic: %q/%q vs %q/%q", firstName, firstDisambiguator, secondName, secondDisambiguator)
	}
	if firstName == "Backup Job" {
		t.Fatal("colliding topic name was not disambiguated")
	}
	if utf8.RuneCountInString(firstName) > telegramtopics.MaxTopicNameLength {
		t.Fatalf("disambiguated topic name length = %d, want <= %d", utf8.RuneCountInString(firstName), telegramtopics.MaxTopicNameLength)
	}
}

func TestTopicNameForSenderScopesCollisionsByRecipientAndChat(t *testing.T) {
	associations := []telegramtopics.Association{
		{RecipientID: "ops", ChatID: "-100", Sender: "Backup\tJob", TopicName: "Backup Job"},
	}

	name, disambiguator := telegramtopics.TopicNameForSender("dev", "-100", "Backup  Job", associations)
	if name != "Backup Job" || disambiguator != "" {
		t.Fatalf("TopicNameForSender(dev) = %q, %q; want no collision", name, disambiguator)
	}

	name, disambiguator = telegramtopics.TopicNameForSender("ops", "-200", "Backup  Job", associations)
	if name != "Backup Job" || disambiguator != "" {
		t.Fatalf("TopicNameForSender(other chat) = %q, %q; want no collision", name, disambiguator)
	}
}
