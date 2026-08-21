package store

import (
	"strings"
	"time"
)

type whereBuilder struct {
	conditions []string
	args       []any
}

func (b *whereBuilder) add(condition string, args ...any) {
	b.conditions = append(b.conditions, condition)
	b.args = append(b.args, args...)
}

func (b *whereBuilder) clause() string {
	if len(b.conditions) == 0 {
		return ""
	}

	return "WHERE " + strings.Join(b.conditions, " AND ")
}

func likeTerm(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

	return "%" + replacer.Replace(value) + "%"
}

func sqliteDatetime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}
