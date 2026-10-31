package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

type notificationEventStore interface {
	ListPendingTagNotificationEvents(ctx context.Context) ([]model.TagNotificationEvent, error)
	MarkTagsEventNotified(ctx context.Context, eventID int64) error
}

type notificationUserStore interface {
	ListUsers(ctx context.Context) ([]model.User, error)
}

type mailer interface {
	Send(ctx context.Context, to []string, subject, textBody, htmlBody string) error
}

type webhookClient interface {
	Send(ctx context.Context, url string, payload []byte) error
}

type Notifications struct {
	events      notificationEventStore
	users       notificationUserStore
	mailer      mailer
	mailEnabled bool
	webhook     webhookClient
	webhookURLs []string
	failures    failureRecorder
}

func NewNotifications(events notificationEventStore, users notificationUserStore, mailer mailer, mailEnabled bool, webhook webhookClient, webhookURLs []string, failures failureRecorder) (*Notifications, error) {
	for _, raw := range webhookURLs {
		parsed, err := url.Parse(raw)

		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("invalid webhook url %q: must be an http or https URL", raw)
		}
	}

	return &Notifications{
		events:      events,
		users:       users,
		mailer:      mailer,
		mailEnabled: mailEnabled,
		webhook:     webhook,
		webhookURLs: webhookURLs,
		failures:    failures,
	}, nil
}

func (n *Notifications) SendPendingTagNotifications(ctx context.Context) (int, error) {
	hasWebhooks := len(n.webhookURLs) > 0

	if !n.mailEnabled && !hasWebhooks {
		return 0, nil
	}

	var recipients []mailRecipient

	if n.mailEnabled {
		var err error

		recipients, err = n.tagNotificationRecipients(ctx)

		if err != nil {
			return 0, err
		}
	}

	if len(recipients) == 0 && !hasWebhooks {
		return 0, nil
	}

	events, err := n.events.ListPendingTagNotificationEvents(ctx)

	if err != nil {
		return 0, err
	}

	n.sendDigest(ctx, mailTargets(recipients, EventTypeUpgradeAvailable), updatesDigest(events))
	n.sendDigest(ctx, mailTargets(recipients, EventTypeTagAdded), tagChangesDigest(events))

	sent := 0

	for _, event := range events {
		if hasWebhooks {
			n.sendWebhooks(ctx, event.ImageEvent)
		}

		if err := n.events.MarkTagsEventNotified(ctx, event.ID); err != nil {
			slog.Error("failed to mark tags event as notified", "eventId", event.ID, "error", err)

			continue
		}

		sent++
	}

	return sent, nil
}

func (n *Notifications) sendDigest(ctx context.Context, to []string, digest tagDigest) {
	if len(to) == 0 || len(digest.Sections) == 0 {
		return
	}

	textBody, htmlBody := digest.render()

	if err := n.mailer.Send(ctx, to, digest.Subject, textBody, htmlBody); err != nil {
		slog.Error("failed to send tag notification email", "subject", digest.Subject, "error", err)
		n.failures.Record(ctx, FailureSourceEmail, "", err)
	}
}

func (n *Notifications) sendWebhooks(ctx context.Context, event model.ImageEvent) {
	payload, err := tagNotificationWebhookPayload(event)

	if err != nil {
		slog.Error("failed to build tag notification webhook payload", "eventId", event.ID, "error", err)

		return
	}

	for _, webhookURL := range n.webhookURLs {
		if err := n.webhook.Send(ctx, webhookURL, payload); err != nil {
			slog.Error("failed to send tag notification webhook", "eventId", event.ID, "url", webhookURL, "error", err)
			n.failures.Record(ctx, FailureSourceWebhook, webhookURL, err)
		}
	}
}

type tagWebhookPayload struct {
	Event           string    `json:"event"`
	ImageName       string    `json:"imageName"`
	Tags            []string  `json:"tags"`
	UpdateAvailable bool      `json:"updateAvailable"`
	OccurredAt      time.Time `json:"occurredAt"`
}

func tagNotificationWebhookPayload(event model.ImageEvent) ([]byte, error) {
	return json.Marshal(tagWebhookPayload{
		Event:           event.Type,
		ImageName:       event.ImageName,
		Tags:            event.Data.Tags,
		UpdateAvailable: event.Type == EventTypeUpgradeAvailable,
		OccurredAt:      event.CreatedAt,
	})
}

type mailRecipient struct {
	email string
	level model.NotifyLevel
}

func (n *Notifications) tagNotificationRecipients(ctx context.Context) ([]mailRecipient, error) {
	users, err := n.users.ListUsers(ctx)

	if err != nil {
		return nil, err
	}

	recipients := make([]mailRecipient, 0, len(users))

	for _, user := range users {
		if user.Email == "" || user.Preferences.NotifyLevel == model.NotifyLevelNone {
			continue
		}

		recipients = append(recipients, mailRecipient{email: user.Email, level: user.Preferences.NotifyLevel})
	}

	return recipients, nil
}

func mailTargets(recipients []mailRecipient, eventType string) []string {
	to := make([]string, 0, len(recipients))

	for _, recipient := range recipients {
		if recipient.level == model.NotifyLevelAll || eventType == EventTypeUpgradeAvailable {
			to = append(to, recipient.email)
		}
	}

	return to
}
