package tagfilter

import (
	_ "embed"
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

//go:embed filters.yaml
var defaultFiltersYAML []byte

type fileConfig struct {
	TagFilters struct {
		Exclude []string `yaml:"exclude"`
	} `yaml:"tag_filters"`
}

type Filter struct {
	exclude []*regexp.Regexp
}

func Load(path string) (*Filter, error) {
	data := defaultFiltersYAML

	if path != "" {
		fileData, err := os.ReadFile(path)

		if err != nil {
			return nil, fmt.Errorf("reading filters: %w", err)
		}

		data = fileData
	}

	var cfg fileConfig

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing filters: %w", err)
	}

	exclude := make([]*regexp.Regexp, 0, len(cfg.TagFilters.Exclude))

	for _, pattern := range cfg.TagFilters.Exclude {
		re, err := regexp.Compile(pattern)

		if err != nil {
			return nil, fmt.Errorf("invalid tag_filters.exclude pattern %q: %w", pattern, err)
		}

		exclude = append(exclude, re)
	}

	return &Filter{exclude: exclude}, nil
}

func (f *Filter) Apply(tags []string) []string {
	if len(f.exclude) == 0 {
		return tags
	}

	filtered := make([]string, 0, len(tags))

	for _, tag := range tags {
		if f.excluded(tag) {
			continue
		}

		filtered = append(filtered, tag)
	}

	return filtered
}

func (f *Filter) excluded(tag string) bool {
	for _, re := range f.exclude {
		if re.MatchString(tag) {
			return true
		}
	}

	return false
}
