package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
)

type fakeImageTags struct {
	imageService
	image model.Image
	tags  []model.ImageTag
}

func (f fakeImageTags) GetByID(context.Context, int64) (*model.Image, error) {
	return &f.image, nil
}

func (f fakeImageTags) GetTags(context.Context, int64) ([]model.ImageTag, error) {
	return f.tags, nil
}

func getImageTagsFor(t *testing.T, url string) *httptest.ResponseRecorder {
	t.Helper()

	w := &Web{images: fakeImageTags{
		image: model.Image{ID: 1, Tag: "1.1"},
		tags: []model.ImageTag{
			{Tag: "2.0-alpine", FamilyID: 7, TagOrder: 0},
			{Tag: "1.1-alpine", FamilyID: 7, TagOrder: 1},
			{Tag: "1.2", FamilyID: 3, TagOrder: 2},
			{Tag: "1.2-rc1", FamilyID: 3, TagOrder: 3, Prerelease: true},
			{Tag: "1.1", FamilyID: 3, TagOrder: 4},
			{Tag: "1.0", FamilyID: 3, TagOrder: 5},
		},
	}}

	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("id", "1")

	rec := httptest.NewRecorder()
	w.apiGetImageTags(rec, req)

	return rec
}

func tagNames(t *testing.T, rec *httptest.ResponseRecorder) [][]string {
	t.Helper()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	var resp imageTagsResponse

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	groups := make([][]string, 0, len(resp.TagGroups))

	for _, group := range resp.TagGroups {
		names := make([]string, 0, len(group.Tags))

		for _, tag := range group.Tags {
			names = append(names, tag.Tag)
		}

		groups = append(groups, names)
	}

	return groups
}

func TestGetImageTagsDefaultReturnsEveryFamily(t *testing.T) {
	got := tagNames(t, getImageTagsFor(t, "/images/1/tags"))
	want := [][]string{{"1.2", "1.2-rc1", "1.1", "1.0"}, {"2.0-alpine", "1.1-alpine"}}

	if !slices.EqualFunc(got, want, slices.Equal[[]string]) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestGetImageTagsScopeNewer(t *testing.T) {
	got := tagNames(t, getImageTagsFor(t, "/images/1/tags?scope=newer"))
	want := [][]string{{"1.2", "1.2-rc1", "1.1"}}

	if !slices.EqualFunc(got, want, slices.Equal[[]string]) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestGetImageTagsInvalidScope(t *testing.T) {
	rec := getImageTagsFor(t, "/images/1/tags?scope=older")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
