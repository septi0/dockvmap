package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

type whereBuilder struct {
	conditions []string
	args       []any
}

func (b *whereBuilder) add(condition string, args ...any) {
	b.conditions = append(b.conditions, condition)
	b.args = append(b.args, args...)
}

func (b *whereBuilder) dateRange(column string, since, until *time.Time) {
	if since != nil {
		b.add(column+" >= ?", sqliteDatetime(*since))
	}

	if until != nil {
		b.add(column+" <= ?", sqliteDatetime(*until))
	}
}

func (b *whereBuilder) clause() string {
	if len(b.conditions) == 0 {
		return ""
	}

	return "WHERE " + strings.Join(b.conditions, " AND ")
}

type sortColumn struct {
	expr     string
	nullable bool
}

type sortSpec struct {
	columns  map[string]sortColumn
	fallback string
	tieBreak string
}

func (s sortSpec) orderBy(sort model.Sort) string {
	column, ok := s.columns[sort.Field]

	if !ok {
		return "ORDER BY " + s.fallback
	}

	direction := "ASC"

	if sort.Direction == model.SortDescending {
		direction = "DESC"
	}

	var terms []string

	if column.nullable {
		terms = append(terms, column.expr+" IS NULL")
	}

	terms = append(terms, column.expr+" "+direction, s.tieBreak)

	return "ORDER BY " + strings.Join(terms, ", ")
}

func (s *Store) countWhere(ctx context.Context, from, where string, args []any) (int64, error) {
	var count int64

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", from, where)

	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting %s: %w", from, err)
	}

	return count, nil
}

func likeTerm(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

	return "%" + replacer.Replace(value) + "%"
}

func sqliteDatetime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}
