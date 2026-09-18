package config

import "testing"

// TestValidateTopicFilter is white-box because validateTopicFilter is unexported, and
// because it is the only place the NUL rule can be tested: an environment variable cannot
// hold a NUL byte, so LoadConfig never sees one.
func TestValidateTopicFilter(t *testing.T) {
	tests := []struct {
		name    string
		filter  string
		wantErr bool
	}{
		{"multi-level wildcard alone (§4.7.1.2)", "#", false},
		{"single-level wildcard alone (§4.7.1.3)", "+", false},
		{"trailing multi-level wildcard (§4.7.1.2)", "sport/tennis/#", false},
		{"single-level wildcard in the middle (§4.7.1.3)", "sport/+/player1", false},
		{"both wildcards (§4.7.1.3)", "+/tennis/#", false},
		{"two single-level wildcards", "+/+", false},
		{"single-level wildcard after an empty level", "/+", false},
		{"single-level then multi-level wildcard", "+/#", false},
		{"separator only", "/", false},
		{"empty middle level", "a//b", false},
		{"empty last level", "a/", false},
		{"system topic with multi-level wildcard (§4.7.2)", "$SYS/#", false},
		{"system topic with single-level wildcard (§4.7.2)", "$SYS/monitor/+", false},
		{"zigbee2mqtt bridge", "zigbee2mqtt/bridge/#", false},
		{"space inside a level", "living room/+", false},
		{"multi-level wildcard glued to a level (§4.7.1.2)", "sport/tennis#", true},
		{"multi-level wildcard not last (§4.7.1.2)", "sport/tennis/#/ranking", true},
		{"multi-level wildcard followed by an empty level", "#/", true},
		{"doubled multi-level wildcard", "##", true},
		{"multi-level wildcard at the end of a level", "a/b#", true},
		{"single-level wildcard glued to a level (§4.7.1.3)", "sport+", true},
		{"single-level wildcard at the start of a level", "+a/b", true},
		{"single-level wildcard before text", "a/+b", true},
		{"single-level wildcard after text", "a/b+", true},
		{"doubled single-level wildcard", "++", true},
		{"NUL inside a level", "a\x00b", true},
		{"NUL alone", "\x00", true},
		{"NUL after a valid filter", "a/#\x00", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTopicFilter(tt.filter)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTopicFilter(%q) error = %v, wantErr %v", tt.filter, err, tt.wantErr)
			}
		})
	}
}
