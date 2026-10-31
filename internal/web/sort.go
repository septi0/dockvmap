package web

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/septi0/dockvmap/internal/model"
)

func parseSort(r *http.Request, fields ...string) (model.Sort, error) {
	field := r.URL.Query().Get("sort")
	direction := model.SortDirection(r.URL.Query().Get("order"))

	switch direction {
	case "":
		direction = model.SortAscending
	case model.SortAscending, model.SortDescending:
	default:
		return model.Sort{}, fmt.Errorf("order must be asc or desc")
	}

	if field == "" {
		return model.Sort{}, nil
	}

	if !slices.Contains(fields, field) {
		return model.Sort{}, fmt.Errorf("sort must be one of: %s", strings.Join(fields, ", "))
	}

	return model.Sort{Field: field, Direction: direction}, nil
}
