package service

import (
	"cmp"
	"embed"
	"fmt"
	"html/template"
	"slices"
	"strconv"
	"strings"

	"github.com/septi0/dockvmap/internal/model"
)

const digestMaxTags = 5

type digestLine struct {
	Name string
	Tags string
}

type digestSection struct {
	Title string
	Lines []digestLine
}

type tagDigest struct {
	Subject  string
	Heading  string
	Sections []digestSection
}

//go:embed templates/tag_notification.html
var tagEmailTemplateFS embed.FS

var tagEmailTemplate = template.Must(template.ParseFS(tagEmailTemplateFS, "templates/tag_notification.html"))

func updatesDigest(events []model.TagNotificationEvent) tagDigest {
	lines := digestLines(eventsOfType(events, EventTypeUpgradeAvailable), true)

	if len(lines) == 0 {
		return tagDigest{}
	}

	noun := "updates"

	if len(lines) == 1 {
		noun = "update"
	}

	heading := fmt.Sprintf("%d %s available", len(lines), noun)

	return tagDigest{
		Subject:  "dockvmap: " + heading,
		Heading:  heading,
		Sections: []digestSection{{Lines: lines}},
	}
}

func tagChangesDigest(events []model.TagNotificationEvent) tagDigest {
	added := eventsOfType(events, EventTypeTagAdded)
	removed := eventsOfType(events, EventTypeTagRemoved)

	var sections []digestSection

	if lines := digestLines(added, false); len(lines) > 0 {
		sections = append(sections, digestSection{Title: "New tags", Lines: lines})
	}

	if lines := digestLines(removed, false); len(lines) > 0 {
		sections = append(sections, digestSection{Title: "Removed tags", Lines: lines})
	}

	if len(sections) == 0 {
		return tagDigest{}
	}

	images := map[int64]struct{}{}

	for _, event := range slices.Concat(added, removed) {
		images[event.ImageID] = struct{}{}
	}

	heading := "Tag changes for " + imageCount(len(images))

	return tagDigest{
		Subject:  "dockvmap: tag changes for " + imageCount(len(images)),
		Heading:  heading,
		Sections: sections,
	}
}

func eventsOfType(events []model.TagNotificationEvent, eventType string) []model.TagNotificationEvent {
	matching := make([]model.TagNotificationEvent, 0, len(events))

	for _, event := range events {
		if event.Type == eventType {
			matching = append(matching, event)
		}
	}

	return matching
}

type digestGroupBucket struct {
	name   string
	size   int
	tags   []string
	images []string
}

func digestLines(events []model.TagNotificationEvent, latestOnly bool) []digestLine {
	imageTags := map[int64][]string{}
	first := map[int64]model.TagNotificationEvent{}
	var order []int64

	for _, event := range events {
		if _, seen := first[event.ImageID]; !seen {
			first[event.ImageID] = event
			order = append(order, event.ImageID)
		}

		if latestOnly {
			imageTags[event.ImageID] = event.Data.Tags
			continue
		}

		for _, tag := range event.Data.Tags {
			if !slices.Contains(imageTags[event.ImageID], tag) {
				imageTags[event.ImageID] = append(imageTags[event.ImageID], tag)
			}
		}
	}

	lines := make([]digestLine, 0, len(order))
	buckets := map[string]*digestGroupBucket{}

	for _, imageID := range order {
		event := first[imageID]
		tags := imageTags[imageID]

		if event.GroupID == nil {
			lines = append(lines, digestLine{Name: event.ImageName, Tags: digestTagList(tags)})
			continue
		}

		key := strconv.FormatInt(*event.GroupID, 10) + "\x00" + strings.Join(tags, "\x00")
		bucket, ok := buckets[key]

		if !ok {
			bucket = &digestGroupBucket{name: event.GroupName, size: event.GroupSize, tags: tags}
			buckets[key] = bucket
		}

		bucket.images = append(bucket.images, event.ImageName)
	}

	for _, bucket := range buckets {
		lines = append(lines, digestLine{Name: bucket.label(), Tags: digestTagList(bucket.tags)})
	}

	slices.SortFunc(lines, func(a, b digestLine) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Tags, b.Tags))
	})

	return lines
}

func (b *digestGroupBucket) label() string {
	if len(b.images) >= b.size {
		return fmt.Sprintf("%s (%s)", b.name, imageCount(b.size))
	}

	slices.Sort(b.images)

	return fmt.Sprintf("%s (%d of %d images: %s)", b.name, len(b.images), b.size, strings.Join(b.images, ", "))
}

func imageCount(count int) string {
	if count == 1 {
		return "1 image"
	}

	return fmt.Sprintf("%d images", count)
}

func digestTagList(tags []string) string {
	if len(tags) <= digestMaxTags {
		return strings.Join(tags, ", ")
	}

	return fmt.Sprintf("%s, +%d more", strings.Join(tags[:digestMaxTags], ", "), len(tags)-digestMaxTags)
}

func (d tagDigest) render() (textBody, htmlBody string) {
	var text strings.Builder

	text.WriteString(d.Heading + ":\n")

	for _, section := range d.Sections {
		text.WriteString("\n")

		if section.Title != "" {
			text.WriteString(section.Title + "\n")
		}

		for _, line := range section.Lines {
			text.WriteString("  " + line.Name + ": " + line.Tags + "\n")
		}
	}

	text.WriteString("\nThis was detected automatically by dockvmap's tag refresh.")

	var buf strings.Builder

	if err := tagEmailTemplate.Execute(&buf, d); err == nil {
		htmlBody = buf.String()
	}

	return text.String(), htmlBody
}
