package config_test

import (
	"testing"

	"github.com/spydemon/mqtt2bdd/internal/config"
)

func TestLoadConfig(t *testing.T) {
	allRequired := map[string]string{
		"MQTT_BROKER":       "mqtt.test.local",
		"MQTT_PORT":         "1883",
		"POSTGRES_HOST":     "postgres.test.local",
		"POSTGRES_PORT":     "5432",
		"POSTGRES_DB":       "testdb",
		"POSTGRES_USER":     "testuser",
		"POSTGRES_PASSWORD": "testpassword",
	}

	varToDeleteBeforeTests := []string{
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
	}

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
			if !tt.wantErr && *got != *tt.want {
				t.Errorf("LoadConfig() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
