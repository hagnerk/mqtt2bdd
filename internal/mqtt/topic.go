package mqtt

import "strings"

const (
	topicLevelSeparator = "/"
	multiLevelWildcard  = "#"
	singleLevelWildcard = "+"
	systemTopicPrefix   = "$"
)

// MatchTopic reports whether topic matches filter under the MQTT 3.1.1 §4.7 rules: "+"
// matches exactly one level, "#" matches the parent level and any number of child levels
// ("a/#" matches "a"), empty levels are significant, and the comparison is case-sensitive.
// Per §4.7.2, a topic starting with "$" is never matched by a filter starting with a
// wildcard.
//
// filter must be a valid topic filter, as internal/config guarantees for
// MQTT_EXCLUDE_TOPICS; for an invalid one the result is unspecified, but MatchTopic never
// panics. It walks both strings with strings.Cut rather than splitting them, so it
// allocates nothing: it runs for every configured filter on every received message.
func MatchTopic(filter, topic string) bool {
	if strings.HasPrefix(topic, systemTopicPrefix) &&
		(strings.HasPrefix(filter, singleLevelWildcard) || strings.HasPrefix(filter, multiLevelWildcard)) {
		return false
	}
	for {
		filterLevel, filterRest, filterHasMore := strings.Cut(filter, topicLevelSeparator)
		if filterLevel == multiLevelWildcard {
			return true
		}
		topicLevel, topicRest, topicHasMore := strings.Cut(topic, topicLevelSeparator)
		if filterLevel != singleLevelWildcard && filterLevel != topicLevel {
			return false
		}
		switch {
		case !filterHasMore && !topicHasMore:
			return true
		case !filterHasMore:
			return false
		case !topicHasMore:
			// Only a trailing "#" matches the parent level: "a/#" matches "a", "a/+" does not.
			return filterRest == multiLevelWildcard
		}
		filter, topic = filterRest, topicRest
	}
}
