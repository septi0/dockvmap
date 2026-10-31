package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

type fakeImageGroups struct {
	imageService
	err    error
	filter *model.ImageGroupListFilters
	ids    *[]int64
}

func (f fakeImageGroups) UpdateTag(context.Context, int64, string, model.TagHistorySource) error {
	return f.err
}

func (f fakeImageGroups) SetPinned(context.Context, int64, bool) error {
	return f.err
}

func (f fakeImageGroups) UpdateGroup(context.Context, int64, service.ImageGroupInput) error {
	return f.err
}

func (f fakeImageGroups) CreateGroup(context.Context, service.ImageGroupInput) (int64, error) {
	return 7, f.err
}

func (f fakeImageGroups) GetGroup(context.Context, int64) (*model.ImageGroup, error) {
	return nil, f.err
}

func (f fakeImageGroups) ListGroups(_ context.Context, filters model.ImageGroupListFilters) ([]model.ImageGroup, error) {
	if f.filter != nil {
		*f.filter = filters
	}

	return nil, f.err
}

func (f fakeImageGroups) CountGroups(context.Context, model.ImageGroupListFilters) (int64, error) {
	return 0, nil
}

func (f fakeImageGroups) CommonTags(_ context.Context, ids []int64) ([]model.ImageTag, error) {
	if f.ids != nil {
		*f.ids = ids
	}

	return nil, f.err
}

func request(method, url, body string) *http.Request {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")

	return req
}

func TestImageGroupErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		body string
	}{
		{"not found", fmt.Errorf("%w: 1", service.ErrImageGroupNotFound), http.StatusNotFound, "image group not found"},
		{"image not found", fmt.Errorf("%w: 1", service.ErrImageNotFound), http.StatusNotFound, "image not found"},
		{"name taken", service.ErrImageGroupAlreadyExists, http.StatusConflict, "image group name already exists"},
		{"invalid", fmt.Errorf("%w: name is required", service.ErrInvalidImageGroup), http.StatusBadRequest, "name is required"},
		{"busy", service.ErrImageGroupBusy, http.StatusConflict, "try again shortly"},
		{"other group", fmt.Errorf("%w: a, b", service.ErrImageInAnotherGroup), http.StatusConflict, "a, b"},
		{"tag missing", &service.GroupTagUnavailableError{Tag: "1.1", Images: []string{"redis"}}, http.StatusConflict, "redis"},
		{"unexpected", errors.New("db is down"), http.StatusInternalServerError, "internal server error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &Web{images: fakeImageGroups{err: tc.err}}
			rec := httptest.NewRecorder()

			w.apiUpdateImageGroup(rec, request(http.MethodPut, "/image-groups/1", `{"name":"x","imageIds":[1,2],"tag":"1.1"}`))

			if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.body) {
				t.Fatalf("got %d %s, want %d containing %q", rec.Code, rec.Body, tc.code, tc.body)
			}
		})
	}
}

func TestUpdateImageTagNamesImagesMissingAGroupTag(t *testing.T) {
	w := &Web{images: fakeImageGroups{err: &service.GroupTagUnavailableError{Tag: "1.1", Images: []string{"redis"}}}}
	rec := httptest.NewRecorder()

	w.apiUpdateImageTag(rec, request(http.MethodPut, "/images/1/tag", `{"tag":"1.1"}`))

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "redis") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestUpdateImageTagKeepsPlainConflictForUngroupedImages(t *testing.T) {
	w := &Web{images: fakeImageGroups{err: fmt.Errorf("%w: %q", service.ErrTagUnavailable, "1.1")}}
	rec := httptest.NewRecorder()

	w.apiUpdateImageTag(rec, request(http.MethodPut, "/images/1/tag", `{"tag":"1.1"}`))

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "image tag is not available") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestSetImagePinReportsBusyGroup(t *testing.T) {
	w := &Web{images: fakeImageGroups{err: service.ErrImageGroupBusy}}
	rec := httptest.NewRecorder()

	w.apiSetImagePin(rec, request(http.MethodPut, "/images/1/pin", `{"pinned":true}`))

	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestCreateImageGroupReturnsID(t *testing.T) {
	w := &Web{images: fakeImageGroups{}}
	rec := httptest.NewRecorder()

	w.apiCreateImageGroup(rec, request(http.MethodPost, "/image-groups", `{"name":"x","imageIds":[1,2],"tag":"1.1"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}

	var body struct {
		ID int64 `json:"id"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.ID != 7 {
		t.Fatalf("body = %s (%v)", rec.Body, err)
	}
}

func TestListImageGroupsFilters(t *testing.T) {
	var filters model.ImageGroupListFilters

	w := &Web{images: fakeImageGroups{filter: &filters}}
	rec := httptest.NewRecorder()

	w.apiListImageGroups(rec, request(http.MethodGet, "/image-groups?status=updateAvailable&search=cloud&limit=5", ""))

	if rec.Code != http.StatusOK || !filters.UpdateAvailable || filters.Search != "cloud" || filters.Limit != 5 {
		t.Fatalf("got %d, filters %+v", rec.Code, filters)
	}

	rec = httptest.NewRecorder()

	w.apiListImageGroups(rec, request(http.MethodGet, "/image-groups?status=bogus", ""))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus status: got %d", rec.Code)
	}
}

func TestCommonTagsParsesImageIDs(t *testing.T) {
	var ids []int64

	w := &Web{images: fakeImageGroups{ids: &ids}}
	rec := httptest.NewRecorder()

	w.apiGetCommonTags(rec, request(http.MethodGet, "/image-groups/common-tags?imageIds=3,5", ""))

	if rec.Code != http.StatusOK || !slices.Equal(ids, []int64{3, 5}) {
		t.Fatalf("got %d, ids %v", rec.Code, ids)
	}

	for _, url := range []string{"/image-groups/common-tags", "/image-groups/common-tags?imageIds=a,b"} {
		rec := httptest.NewRecorder()

		w.apiGetCommonTags(rec, request(http.MethodGet, url, ""))

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: got %d", url, rec.Code)
		}
	}
}

func TestListImagesUngroupedFilter(t *testing.T) {
	filters, err := parseImageListFilters(request(http.MethodGet, "/images?ungrouped=true", ""))

	if err != nil || !filters.Ungrouped {
		t.Fatalf("filters = %+v (%v)", filters, err)
	}

	filters, err = parseImageListFilters(request(http.MethodGet, "/images", ""))

	if err != nil || filters.Ungrouped {
		t.Fatalf("filters = %+v (%v)", filters, err)
	}
}

func TestListImagesGroupFilter(t *testing.T) {
	filters, err := parseImageListFilters(request(http.MethodGet, "/images?groupId=3", ""))

	if err != nil || filters.GroupID != 3 {
		t.Fatalf("filters = %+v (%v)", filters, err)
	}

	for _, value := range []string{"abc", "0", "-1"} {
		if _, err := parseImageListFilters(request(http.MethodGet, "/images?groupId="+value, "")); err == nil {
			t.Fatalf("groupId=%s: want an error", value)
		}
	}
}

func TestListImageGroupsPinnedFilter(t *testing.T) {
	filters, err := parseImageGroupFilters(request(http.MethodGet, "/image-groups?status=pinned", ""))

	if err != nil || !filters.Pinned || filters.UpdateAvailable {
		t.Fatalf("filters = %+v (%v)", filters, err)
	}
}

func TestNewImageResponseOmitsGroupForUngroupedImages(t *testing.T) {
	encoded, err := json.Marshal(newImageResponse(model.Image{ID: 1}))

	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(encoded), `"group"`) {
		t.Fatalf("response = %s", encoded)
	}

	id, name := int64(4), "stack"

	response := newImageResponse(model.Image{ID: 1, GroupID: &id, GroupName: &name})

	if response.Group == nil || response.Group.ID != 4 || response.Group.Name != "stack" {
		t.Fatalf("group = %+v", response.Group)
	}
}
