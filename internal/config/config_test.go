package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hagnerk/mqtt2bdd/internal/config"
)

// allRequired sets every required variable to a valid value.
var allRequired = map[string]string{
	"MQTT_BROKER":       "mqtt.test.local",
	"MQTT_PORT":         "1883",
	"POSTGRES_HOST":     "postgres.test.local",
	"POSTGRES_PORT":     "5432",
	"POSTGRES_DB":       "testdb",
	"POSTGRES_USER":     "testuser",
	"POSTGRES_PASSWORD": "testpassword",
}

// varToDeleteBeforeTests lists every variable LoadConfig reads. Each test row resets them
// all, so no row depends on what the go-dev container's environment happens to hold.
var varToDeleteBeforeTests = []string{
	"MQTT_BROKER",
	"MQTT_PORT",
	"MQTT_USERNAME",
	"MQTT_PASSWORD",
	"POSTGRES_HOST",
	"POSTGRES_PORT",
	"POSTGRES_DB",
	"POSTGRES_USER",
	"POSTGRES_PASSWORD",
	"LOG_LEVEL",
	"BUFFER_SIZE",
	"MQTT_EXCLUDE_TOPICS",
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    *config.Config
		wantErr bool
	}{
		{
			name: "all required vars set with defaults",
			env:  allRequired,
			want: &config.Config{
				MQTTBroker:       "mqtt.test.local",
				MQTTPort:         1883,
				MQTTUsername:     "",
				MQTTPassword:     "",
				PostgresHost:     "postgres.test.local",
				PostgresPort:     5432,
				PostgresDB:       "testdb",
				PostgresUser:     "testuser",
				PostgresPassword: "testpassword",
				LogLevel:         "INFO",
				BufferSize:       1000,
			},
			wantErr: false,
		},
		{
			name: "missing MQTT_BROKER",
			env: map[string]string{
				"MQTT_PORT":         "1883",
				"POSTGRES_HOST":     "postgres.test.local",
				"POSTGRES_PORT":     "5432",
				"POSTGRES_DB":       "testdb",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpassword",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "missing MQTT_PORT",
			env: map[string]string{
				"MQTT_BROKER":       "mqtt.test.local",
				"POSTGRES_HOST":     "postgres.test.local",
				"POSTGRES_PORT":     "5432",
				"POSTGRES_DB":       "testdb",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpassword",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "missing POSTGRES_HOST",
			env: map[string]string{
				"MQTT_BROKER":       "mqtt.test.local",
				"MQTT_PORT":         "1883",
				"POSTGRES_PORT":     "5432",
				"POSTGRES_DB":       "testdb",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpassword",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "missing POSTGRES_PORT",
			env: map[string]string{
				"MQTT_BROKER":       "mqtt.test.local",
				"MQTT_PORT":         "1883",
				"POSTGRES_HOST":     "postgres.test.local",
				"POSTGRES_DB":       "testdb",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpassword",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "missing POSTGRES_DB",
			env: map[string]string{
				"MQTT_BROKER":       "mqtt.test.local",
				"MQTT_PORT":         "1883",
				"POSTGRES_HOST":     "postgres.test.local",
				"POSTGRES_PORT":     "5432",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpassword",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "missing POSTGRES_USER",
			env: map[string]string{
				"MQTT_BROKER":       "mqtt.test.local",
				"MQTT_PORT":         "1883",
				"POSTGRES_HOST":     "postgres.test.local",
				"POSTGRES_PORT":     "5432",
				"POSTGRES_DB":       "testdb",
				"POSTGRES_PASSWORD": "testpassword",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "missing POSTGRES_PASSWORD",
			env: map[string]string{
				"MQTT_BROKER":   "mqtt.test.local",
				"MQTT_PORT":     "1883",
				"POSTGRES_HOST": "postgres.test.local",
				"POSTGRES_PORT": "5432",
				"POSTGRES_DB":   "testdb",
				"POSTGRES_USER": "testuser",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid MQTT_PORT non-numeric",
			env: map[string]string{
				"MQTT_BROKER":       "mqtt.test.local",
				"MQTT_PORT":         "not-a-port",
				"POSTGRES_HOST":     "postgres.test.local",
				"POSTGRES_PORT":     "5432",
				"POSTGRES_DB":       "testdb",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpassword",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid POSTGRES_PORT non-numeric",
			env: map[string]string{
				"MQTT_BROKER":       "mqtt.test.local",
				"MQTT_PORT":         "1883",
				"POSTGRES_HOST":     "postgres.test.local",
				"POSTGRES_PORT":     "not-a-port",
				"POSTGRES_DB":       "testdb",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpassword",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "custom BUFFER_SIZE",
			env: map[string]string{
				"MQTT_BROKER":       "mqtt.test.local",
				"MQTT_PORT":         "1883",
				"POSTGRES_HOST":     "postgres.test.local",
				"POSTGRES_PORT":     "5432",
				"POSTGRES_DB":       "testdb",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpassword",
				"BUFFER_SIZE":       "5000",
			},
			want: &config.Config{
				MQTTBroker:       "mqtt.test.local",
				MQTTPort:         1883,
				MQTTUsername:     "",
				MQTTPassword:     "",
				PostgresHost:     "postgres.test.local",
				PostgresPort:     5432,
				PostgresDB:       "testdb",
				PostgresUser:     "testuser",
				PostgresPassword: "testpassword",
				LogLevel:         "INFO",
				BufferSize:       5000,
			},
			wantErr: false,
		},
		{
			name: "invalid BUFFER_SIZE non-numeric",
			env: map[string]string{
				"MQTT_BROKER":       "mqtt.test.local",
				"MQTT_PORT":         "1883",
				"POSTGRES_HOST":     "postgres.test.local",
				"POSTGRES_PORT":     "5432",
				"POSTGRES_DB":       "testdb",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_PASSWORD": "testpassword",
				"BUFFER_SIZE":       "not-a-number",
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range varToDeleteBeforeTests {
				t.Setenv(v, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := config.LoadConfig()
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadConfig() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadConfig_ExcludeTopics(t *testing.T) {
	tests := []struct {
		name string
		// value is applied only when set is true, so the "unset" row keeps the reset value.
		set       bool
		value     string
		want      []string
		wantErrOn string
	}{
		{name: "unset excludes nothing", set: false, want: nil},
		{name: "empty excludes nothing", set: true, value: "", want: nil},
		{name: "blanks only exclude nothing", set: true, value: "   ", want: nil},
		{name: "separators only exclude nothing", set: true, value: ", ,,", want: nil},
		{name: "one filter", set: true, value: "zigbee2mqtt/bridge/#", want: []string{"zigbee2mqtt/bridge/#"}},
		{name: "trimming and empty entries", set: true, value: " a/# , b/+ ,,c ", want: []string{"a/#", "b/+", "c"}},
		{name: "duplicates kept in order", set: true, value: "a/#,a/#", want: []string{"a/#", "a/#"}},
		{name: "inner spaces kept", set: true, value: " living room/+ ", want: []string{"living room/+"}},
		{name: "invalid filter only", set: true, value: "sport/tennis/#/ranking", wantErrOn: "sport/tennis/#/ranking"},
		{name: "valid filter then invalid one", set: true, value: "ok/#, sport+", wantErrOn: "sport+"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range varToDeleteBeforeTests {
				t.Setenv(v, "")
			}
			for k, v := range allRequired {
				t.Setenv(k, v)
			}
			if tt.set {
				t.Setenv("MQTT_EXCLUDE_TOPICS", tt.value)
			}

			got, err := config.LoadConfig()
			if tt.wantErrOn != "" {
				if err == nil {
					t.Fatalf("LoadConfig() error = nil, want an error naming %q", tt.wantErrOn)
				}
				for _, want := range []string{"MQTT_EXCLUDE_TOPICS", `"` + tt.wantErrOn + `"`} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("LoadConfig() error = %q, want it to contain %q", err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got.ExcludeTopics, tt.want) {
				t.Errorf("ExcludeTopics = %#v, want %#v", got.ExcludeTopics, tt.want)
			}
		})
	}
}
