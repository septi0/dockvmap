package web

import (
	"net/http/httptest"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
)

func TestParseSort(t *testing.T) {
	tests := []struct {
		query   string
		want    model.Sort
		wantErr bool
	}{
		{query: "", want: model.Sort{}},
		{query: "?order=desc", want: model.Sort{}},
		{query: "?sort=name", want: model.Sort{Field: "name", Direction: model.SortAscending}},
		{query: "?sort=name&order=asc", want: model.Sort{Field: "name", Direction: model.SortAscending}},
		{query: "?sort=lastPulled&order=desc", want: model.Sort{Field: "lastPulled", Direction: model.SortDescending}},
		{query: "?sort=id", wantErr: true},
		{query: "?sort=name%3BDROP%20TABLE%20images", wantErr: true},
		{query: "?sort=name&order=sideways", wantErr: true},
		{query: "?order=sideways", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/images"+tt.query, nil)
			got, err := parseSort(r, "name", "lastPulled")

			if tt.wantErr {
				if err == nil {
					t.Fatalf("got %+v, want an error", got)
				}

				return
			}

			if err != nil || got != tt.want {
				t.Fatalf("got %+v, %v, want %+v", got, err, tt.want)
			}
		})
	}
}

func TestListImagesRejectsUnknownSort(t *testing.T) {
	w := &Web{}
	rec := httptest.NewRecorder()

	w.apiListImages(rec, httptest.NewRequest("GET", "/api/images?sort=password_hash", nil))

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
