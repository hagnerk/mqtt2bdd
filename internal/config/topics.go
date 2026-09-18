package config

import (
	"fmt"
	"strings"
)

const (
	topicFilterSeparator = ","
	topicLevelSeparator  = "/"
	multiLevelWildcard   = "#"
	singleLevelWildcard  = "+"
	nulCharacter         = "\x00"
)

// parseTopicFilters turns the raw MQTT_EXCLUDE_TOPICS value into the list of filters it
// names, validating each one so a typo stops the application at startup instead of
// silently excluding nothing.
//
// It returns nil, never an empty slice, when no entry remains: "nothing excluded" then has
// a single representation, which keeps len checks and deep comparisons trivial. Filters
// keep their configured order and duplicates are kept: removing them would add code for
// no behavioural difference, and the first matching filter is the one reported.
func parseTopicFilters(raw string) ([]string, error) {
	var filters []string
	for _, entry := range strings.Split(raw, topicFilterSeparator) {
		filter := strings.TrimSpace(entry)
		if filter == "" {
			continue
		}
		if err := validateTopicFilter(filter); err != nil {
			return nil, fmt.Errorf("MQTT_EXCLUDE_TOPICS: invalid topic filter %q: %w", filter, err)
		}
		filters = append(filters, filter)
	}
	return filters, nil
}

// validateTopicFilter checks the wildcard and NUL rules of MQTT 3.1.1 §4.7.1 and nothing
// more. UTF-8 well-formedness and the 65,535-byte limit of §1.5.3 are deliberately not
// checked: a filter breaking them can never match a topic a broker delivers, so it is
// harmless. It runs once at startup, so splitting into a slice is acceptable here.
func validateTopicFilter(filter string) error {
	if strings.Contains(filter, nulCharacter) {
		return fmt.Errorf("contains a NUL character")
	}
	levels := strings.Split(filter, topicLevelSeparator)
	for i, level := range levels {
		if strings.Contains(level, multiLevelWildcard) && (level != multiLevelWildcard || i != len(levels)-1) {
			return fmt.Errorf("'%s' must be the whole last level", multiLevelWildcard)
		}
		if strings.Contains(level, singleLevelWildcard) && level != singleLevelWildcard {
			return fmt.Errorf("'%s' must be a whole level", singleLevelWildcard)
		}
	}
	return nil
}
