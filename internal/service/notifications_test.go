package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
)

type sentMail struct {
	to      []string
	subject string
	text    string
	html    string
}

type fakeMailer struct {
	sent []sentMail
	err  error
}

func (m *fakeMailer) Send(_ context.Context, to []string, subject, textBody, htmlBody string) error {
	m.sent = append(m.sent, sentMail{to: to, subject: subject, text: textBody, html: htmlBody})

	return m.err
}

type fakeWebhook struct{ payloads []string }

func (w *fakeWebhook) Send(_ context.Context, _ string, payload []byte) error {
	w.payloads = append(w.payloads, string(payload))

	return nil
}

type fakeEventStore struct {
	events   []model.TagNotificationEvent
	notified []int64
}

func (s *fakeEventStore) ListPendingTagNotificationEvents(context.Context) ([]model.TagNotificationEvent, error) {
	return s.events, nil
}

func (s *fakeEventStore) MarkTagsEventNotified(_ context.Context, id int64) error {
	s.notified = append(s.notified, id)

	return nil
}

type fakeUserStore struct{ users []model.User }

func (s fakeUserStore) ListUsers(context.Context) ([]model.User, error) { return s.users, nil }

type countingFailures struct{ sources []FailureSource }

func (f *countingFailures) Record(_ context.Context, source FailureSource, _ string, _ error) {
	f.sources = append(f.sources, source)
}

func notifyUser(email string, level model.NotifyLevel) model.User {
	user := model.User{Email: email}
	user.Preferences.NotifyLevel = level

	return user
}

var groupID = int64(7)

func pendingEvent(id int64, image int64, name, eventType string, tags ...string) model.TagNotificationEvent {
	return model.TagNotificationEvent{
		ImageEvent: model.ImageEvent{ID: id, ImageID: image, ImageName: name, Type: eventType, Data: model.TagsEventData{Tags: tags}},
	}
}

func grouped(event model.TagNotificationEvent, size int) model.TagNotificationEvent {
	event.GroupID = &groupID
	event.GroupName = "Harbor"
	event.GroupSize = size

	return event
}

func newTestNotifications(t *testing.T, events []model.TagNotificationEvent, webhooks []string, users ...model.User) (*Notifications, *fakeEventStore, *fakeMailer, *fakeWebhook, *countingFailures) {
	t.Helper()

	eventStore := &fakeEventStore{events: events}
	mail := &fakeMailer{}
	hook := &fakeWebhook{}
	failures := &countingFailures{}

	notifications, err := NewNotifications(eventStore, fakeUserStore{users: users}, mail, true, hook, webhooks, failures)

	if err != nil {
		t.Fatalf("NewNotifications: %v", err)
	}

	return notifications, eventStore, mail, hook, failures
}

func TestDigestsAreSeparatedByTypeAndLevel(t *testing.T) {
	events := []model.TagNotificationEvent{
		pendingEvent(1, 1, "nginx", EventTypeTagAdded, "1.27.4"),
		pendingEvent(2, 1, "nginx", EventTypeUpgradeAvailable, "1.27.4"),
		pendingEvent(3, 2, "redis", EventTypeTagRemoved, "7.2.0"),
		pendingEvent(4, 3, "grafana", EventTypeUpgradeAvailable, "11.4.0"),
	}

	notifications, eventStore, mail, _, _ := newTestNotifications(t, events, nil,
		notifyUser("all@example.com", model.NotifyLevelAll),
		notifyUser("upgrades@example.com", model.NotifyLevelUpgrades),
		notifyUser("off@example.com", model.NotifyLevelNone),
	)

	sent, err := notifications.SendPendingTagNotifications(context.Background())

	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if sent != 4 || len(eventStore.notified) != 4 {
		t.Fatalf("sent = %d, notified = %v", sent, eventStore.notified)
	}

	if len(mail.sent) != 2 {
		t.Fatalf("emails = %d, want 2", len(mail.sent))
	}

	updates, changes := mail.sent[0], mail.sent[1]

	if updates.subject != "dockvmap: 2 updates available" {
		t.Errorf("updates subject = %q", updates.subject)
	}

	if !slices.Equal(updates.to, []string{"all@example.com", "upgrades@example.com"}) {
		t.Errorf("updates to = %v", updates.to)
	}

	if strings.Contains(updates.text, "redis") || !strings.Contains(updates.text, "grafana: 11.4.0") {
		t.Errorf("updates body = %q", updates.text)
	}

	if changes.subject != "dockvmap: tag changes for 2 images" {
		t.Errorf("changes subject = %q", changes.subject)
	}

	if !slices.Equal(changes.to, []string{"all@example.com"}) {
		t.Errorf("changes to = %v", changes.to)
	}

	for _, want := range []string{"New tags", "Removed tags", "nginx: 1.27.4", "redis: 7.2.0"} {
		if !strings.Contains(changes.text, want) {
			t.Errorf("changes body missing %q: %q", want, changes.text)
		}
	}

	if !strings.Contains(updates.html, "2 updates available") || !strings.Contains(updates.html, "grafana") {
		t.Errorf("updates html did not render: %q", updates.html)
	}
}

func TestDigestSkipsAnEmailWithNothingToSay(t *testing.T) {
	events := []model.TagNotificationEvent{pendingEvent(1, 1, "nginx", EventTypeTagAdded, "1.27.4")}

	notifications, _, mail, _, _ := newTestNotifications(t, events, nil, notifyUser("all@example.com", model.NotifyLevelAll))

	if _, err := notifications.SendPendingTagNotifications(context.Background()); err != nil {
		t.Fatalf("send: %v", err)
	}

	if len(mail.sent) != 1 || mail.sent[0].subject != "dockvmap: tag changes for 1 image" {
		t.Fatalf("mail = %+v", mail.sent)
	}
}

func TestDigestUpgradesOnlyUserGetsNothingForTagChanges(t *testing.T) {
	events := []model.TagNotificationEvent{pendingEvent(1, 1, "nginx", EventTypeTagAdded, "1.27.4")}

	notifications, eventStore, mail, _, _ := newTestNotifications(t, events, nil, notifyUser("upgrades@example.com", model.NotifyLevelUpgrades))

	if _, err := notifications.SendPendingTagNotifications(context.Background()); err != nil {
		t.Fatalf("send: %v", err)
	}

	if len(mail.sent) != 0 {
		t.Fatalf("mail = %+v", mail.sent)
	}

	if len(eventStore.notified) != 1 {
		t.Fatalf("notified = %v", eventStore.notified)
	}
}

func TestDigestGroupLines(t *testing.T) {
	tests := []struct {
		name   string
		events []model.TagNotificationEvent
		want   []digestLine
	}{
		{
			name: "a whole group is one line",
			events: []model.TagNotificationEvent{
				grouped(pendingEvent(1, 1, "core", EventTypeUpgradeAvailable, "v2"), 3),
				grouped(pendingEvent(2, 2, "db", EventTypeUpgradeAvailable, "v2"), 3),
				grouped(pendingEvent(3, 3, "portal", EventTypeUpgradeAvailable, "v2"), 3),
			},
			want: []digestLine{{Name: "Harbor (3 images)", Tags: "v2"}},
		},
		{
			name: "a partial group names its images",
			events: []model.TagNotificationEvent{
				grouped(pendingEvent(1, 2, "db", EventTypeUpgradeAvailable, "v2"), 3),
				grouped(pendingEvent(2, 1, "core", EventTypeUpgradeAvailable, "v2"), 3),
			},
			want: []digestLine{{Name: "Harbor (2 of 3 images: core, db)", Tags: "v2"}},
		},
		{
			name: "members on different tags stay apart",
			events: []model.TagNotificationEvent{
				grouped(pendingEvent(1, 1, "core", EventTypeUpgradeAvailable, "v2"), 2),
				grouped(pendingEvent(2, 2, "db", EventTypeUpgradeAvailable, "v3"), 2),
			},
			want: []digestLine{
				{Name: "Harbor (1 of 2 images: core)", Tags: "v2"},
				{Name: "Harbor (1 of 2 images: db)", Tags: "v3"},
			},
		},
		{
			name: "the newest upgrade for an image wins",
			events: []model.TagNotificationEvent{
				pendingEvent(1, 1, "nginx", EventTypeUpgradeAvailable, "1.27.3"),
				pendingEvent(2, 1, "nginx", EventTypeUpgradeAvailable, "1.27.4"),
			},
			want: []digestLine{{Name: "nginx", Tags: "1.27.4"}},
		},
		{
			name: "lines are sorted by name",
			events: []model.TagNotificationEvent{
				pendingEvent(1, 1, "redis", EventTypeUpgradeAvailable, "8"),
				pendingEvent(2, 2, "Grafana", EventTypeUpgradeAvailable, "12"),
			},
			want: []digestLine{{Name: "Grafana", Tags: "12"}, {Name: "redis", Tags: "8"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := digestLines(tt.events, true); !slices.Equal(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDigestMergesAndLimitsTags(t *testing.T) {
	events := []model.TagNotificationEvent{
		pendingEvent(1, 1, "nginx", EventTypeTagAdded, "a", "b", "c", "d"),
		pendingEvent(2, 1, "nginx", EventTypeTagAdded, "d", "e", "f", "g"),
	}

	want := []digestLine{{Name: "nginx", Tags: "a, b, c, d, e, +2 more"}}

	if got := digestLines(events, false); !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDigestFailedSendIsRecordedAndEventsAreConsumed(t *testing.T) {
	events := []model.TagNotificationEvent{pendingEvent(1, 1, "nginx", EventTypeUpgradeAvailable, "1.27.4")}

	notifications, eventStore, mail, _, failures := newTestNotifications(t, events, nil, notifyUser("all@example.com", model.NotifyLevelAll))
	mail.err = errors.New("smtp down")

	if _, err := notifications.SendPendingTagNotifications(context.Background()); err != nil {
		t.Fatalf("send: %v", err)
	}

	if !slices.Equal(failures.sources, []FailureSource{FailureSourceEmail}) {
		t.Fatalf("failures = %v", failures.sources)
	}

	if len(eventStore.notified) != 1 {
		t.Fatalf("notified = %v", eventStore.notified)
	}
}

func TestWebhooksStayPerImage(t *testing.T) {
	events := []model.TagNotificationEvent{
		grouped(pendingEvent(1, 1, "core", EventTypeUpgradeAvailable, "v2"), 2),
		grouped(pendingEvent(2, 2, "db", EventTypeUpgradeAvailable, "v2"), 2),
	}

	notifications, _, mail, hook, _ := newTestNotifications(t, events, []string{"https://hooks.example.com/x"}, notifyUser("all@example.com", model.NotifyLevelAll))

	if _, err := notifications.SendPendingTagNotifications(context.Background()); err != nil {
		t.Fatalf("send: %v", err)
	}

	if len(mail.sent) != 1 {
		t.Fatalf("emails = %d, want 1", len(mail.sent))
	}

	if len(hook.payloads) != 2 || !strings.Contains(hook.payloads[0], `"imageName":"core"`) || !strings.Contains(hook.payloads[1], `"imageName":"db"`) {
		t.Fatalf("webhooks = %v", hook.payloads)
	}
}

func TestDigestEscapesHTML(t *testing.T) {
	events := []model.TagNotificationEvent{pendingEvent(1, 1, "<script>", EventTypeUpgradeAvailable, "1.0")}

	_, html := updatesDigest(events).render()

	if strings.Contains(html, "<script>") || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("html was not escaped")
	}
}

func TestPendingEventsCarryGroupAndAreNotCapped(t *testing.T) {
	f := newGroupFixture(t)

	ids := []int64{f.addImage("core", "v1", "v1"), f.addImage("db", "v1", "v1"), f.addImage("solo", "v1", "v1")}

	f.group("Harbor", "v1", ids[0], ids[1])

	const total = 60

	for i := range total {
		if err := f.store.AddTagsEvent(f.ctx, ids[i%3], EventTypeTagAdded, true, model.TagsEventData{Tags: []string{"v2"}}); err != nil {
			t.Fatalf("adding event: %v", err)
		}
	}

	if err := f.store.AddTagsEvent(f.ctx, ids[0], EventTypeTagAdded, false, model.TagsEventData{Tags: []string{"silent"}}); err != nil {
		t.Fatalf("adding event: %v", err)
	}

	events, err := f.store.ListPendingTagNotificationEvents(f.ctx)

	if err != nil {
		t.Fatalf("listing: %v", err)
	}

	if len(events) != total {
		t.Fatalf("events = %d, want %d", len(events), total)
	}

	if events[0].GroupID == nil || events[0].GroupName != "Harbor" || events[0].GroupSize != 2 {
		t.Fatalf("grouped event = %+v", events[0])
	}

	if events[2].GroupID != nil || events[2].GroupName != "" || events[2].GroupSize != 0 {
		t.Fatalf("ungrouped event = %+v", events[2])
	}
}
