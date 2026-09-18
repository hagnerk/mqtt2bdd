package mqtt_test

import (
	"testing"

	"github.com/hagnerk/mqtt2bdd/internal/mqtt"
)

func TestMatchTopic(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		topic  string
		want   bool
	}{
		{"multi-level wildcard matches the parent level (§4.7.1.2)", "sport/tennis/player1/#", "sport/tennis/player1", true},
		{"multi-level wildcard matches one child level (§4.7.1.2)", "sport/tennis/player1/#", "sport/tennis/player1/ranking", true},
		{"multi-level wildcard matches several child levels (§4.7.1.2)", "sport/tennis/player1/#", "sport/tennis/player1/score/wimbledon", true},
		{"sport/# matches sport (§4.7.1.2)", "sport/#", "sport", true},
		{"lone multi-level wildcard matches any topic", "#", "sport/tennis/player1", true},
		{"lone multi-level wildcard matches a separator-only topic", "#", "/", true},
		{"empty child level is a level", "a/#", "a/", true},
		{"levels, not string prefixes", "a/#", "ab", false},
		{"different first level", "a/#", "b/a", false},
		{"single-level wildcard matches player1 (§4.7.1.3)", "sport/tennis/+", "sport/tennis/player1", true},
		{"single-level wildcard matches player2 (§4.7.1.3)", "sport/tennis/+", "sport/tennis/player2", true},
		{"single-level wildcard matches exactly one level (§4.7.1.3)", "sport/tennis/+", "sport/tennis/player1/ranking", false},
		{"single-level wildcard does not match the parent level (§4.7.1.3)", "sport/+", "sport", false},
		{"single-level wildcard matches an empty level (§4.7.1.3)", "sport/+", "sport/", true},
		{"two single-level wildcards match a leading empty level (§4.7.1.3)", "+/+", "/finance", true},
		{"leading empty level then single-level wildcard (§4.7.1.3)", "/+", "/finance", true},
		{"one single-level wildcard does not match two levels (§4.7.1.3)", "+", "/finance", false},
		{"lone single-level wildcard matches one level", "+", "finance", true},
		{"single-level wildcard in the middle", "sport/+/player1", "sport/tennis/player1", true},
		{"exact match", "a/b", "a/b", true},
		{"trailing separator makes a distinct topic (§4.7.3)", "a/b", "a/b/", false},
		{"empty levels match exactly", "a//b", "a//b", true},
		{"empty levels are significant", "a/b", "a//b", false},
		{"single-level wildcard matches an empty middle level", "a/+/b", "a//b", true},
		{"matching is case-sensitive (§4.7.3)", "Sport/#", "sport/tennis", false},
		{"ACCOUNTS does not match Accounts (§4.7.3)", "ACCOUNTS", "Accounts", false},
		{"multi-level wildcard does not match a $ topic (§4.7.2)", "#", "$SYS/broker/uptime", false},
		{"leading single-level wildcard does not match a $ topic (§4.7.2)", "+/monitor/Clients", "$SYS/monitor/Clients", false},
		{"explicit $SYS filter with multi-level wildcard (§4.7.2)", "$SYS/#", "$SYS/monitor/Clients", true},
		{"explicit $SYS filter with single-level wildcard (§4.7.2)", "$SYS/monitor/+", "$SYS/monitor/Clients", true},
		{"$ rule concerns only the first character of the topic", "a/$SYS", "a/$SYS", true},
		{"any topic starting with $ is protected", "+/b", "$a/b", false},
		{"zigbee2mqtt bridge child", "zigbee2mqtt/bridge/#", "zigbee2mqtt/bridge/definitions", true},
		{"zigbee2mqtt sensor is not excluded", "zigbee2mqtt/bridge/#", "zigbee2mqtt/living_room/temperature", false},
		{"zigbee2mqtt bridge parent level", "zigbee2mqtt/bridge/#", "zigbee2mqtt/bridge", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mqtt.MatchTopic(tt.filter, tt.topic); got != tt.want {
				t.Errorf("MatchTopic(%q, %q) = %v, want %v", tt.filter, tt.topic, got, tt.want)
			}
		})
	}
}
