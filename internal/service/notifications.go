package service

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

const notificationBatchSize = 50

type notificationEventStore interface {
	ListPendingTagNotificationEvents(ctx context.Context, limit int) ([]model.ImageEvent, error)
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

	var recipients []string

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

	events, err := n.events.ListPendingTagNotificationEvents(ctx, notificationBatchSize)

	if err != nil {
		return 0, err
	}

	sent := 0

	for _, event := range events {
		if len(recipients) > 0 {
			subject, textBody, htmlBody := tagNotificationContent(event)

			if err := n.mailer.Send(ctx, recipients, subject, textBody, htmlBody); err != nil {
				slog.Error("failed to send tag notification email", "eventId", event.ID, "error", err)
				n.failures.Record(FailureSourceEmail, "", err)
			}
		}

		if hasWebhooks {
			n.sendWebhooks(ctx, event)
		}

		if err := n.events.MarkTagsEventNotified(ctx, event.ID); err != nil {
			slog.Error("failed to mark tags event as notified", "eventId", event.ID, "error", err)

			continue
		}

		sent++
	}

	return sent, nil
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
			n.failures.Record(FailureSourceWebhook, webhookURL, err)
		}
	}
}

type tagWebhookPayload struct {
	Event      string    `json:"event"`
	ImageName  string    `json:"imageName"`
	Tags       []string  `json:"tags"`
	OccurredAt time.Time `json:"occurredAt"`
}

func tagNotificationWebhookPayload(event model.ImageEvent) ([]byte, error) {
	return json.Marshal(tagWebhookPayload{
		Event:      event.Type,
		ImageName:  event.ImageName,
		Tags:       event.Data.Tags,
		OccurredAt: event.CreatedAt,
	})
}

func (n *Notifications) tagNotificationRecipients(ctx context.Context) ([]string, error) {
	users, err := n.users.ListUsers(ctx)

	if err != nil {
		return nil, err
	}

	recipients := make([]string, 0, len(users))

	for _, user := range users {
		if user.Email == "" || !user.Preferences.NotifyNewTags {
			continue
		}

		recipients = append(recipients, user.Email)
	}

	return recipients, nil
}

type tagEmailData struct {
	Action    string
	ImageName string
	Tags      []string
}

//go:embed templates/tag_notification.html
var tagEmailTemplateFS embed.FS

var tagEmailTemplate = template.Must(template.ParseFS(tagEmailTemplateFS, "templates/tag_notification.html"))

func tagNotificationContent(event model.ImageEvent) (subject, textBody, htmlBody string) {
	action := "New tag(s) discovered"

	if event.Type == EventTypeTagRemoved {
		action = "Tag(s) removed"
	}

	subject = fmt.Sprintf("dockvmap: %s for %s", action, event.ImageName)

	textBody = fmt.Sprintf(
		"%s for virtual image %q:\n\n  %s\n\nThis was detected automatically by dockvmap's tag refresh.",
		action, event.ImageName, strings.Join(event.Data.Tags, ", "),
	)

	var buf strings.Builder

	if err := tagEmailTemplate.Execute(&buf, tagEmailData{Action: action, ImageName: event.ImageName, Tags: event.Data.Tags}); err == nil {
		htmlBody = buf.String()
	}

	return subject, textBody, htmlBody
}
