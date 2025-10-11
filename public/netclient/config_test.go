package netclient

import (
	"net"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "No TCPUserTimeout",
			config: Config{
				TCPUserTimeout: 0,
				KeepAliveConfig: net.KeepAliveConfig{
					Enable: true,
					Idle:   10 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "KeepAlive idle not set",
			config: Config{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Enable: true,
					Idle:   0, // Not set, so no validation error
				},
			},
			wantErr: false,
		},
		{
			name: "KeepAlive idle less than TCPUserTimeout",
			config: Config{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Enable: true,
					Idle:   5 * time.Second, // Invalid
				},
			},
			wantErr: true,
		},
		{
			name: "KeepAlive idle equal to TCPUserTimeout",
			config: Config{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Enable: true,
					Idle:   10 * time.Second, // Invalid
				},
			},
			wantErr: true,
		},
		{
			name: "KeepAlive idle greater than TCPUserTimeout",
			config: Config{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Enable: true,
					Idle:   15 * time.Second, // Valid
				},
			},
			wantErr: false,
		},
		{
			name: "KeepAlive disabled",
			config: Config{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Enable: false, // Disabled
					Idle:   5 * time.Second,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
