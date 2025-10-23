// Copyright 2025 Redpanda Data, Inc.

package netutil

import (
	"net"
	"testing"
	"time"
)

func TestDialerConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  DialerConfig
		wantErr bool
	}{
		{
			name: "No TCPUserTimeout",
			config: DialerConfig{
				TCPUserTimeout: 0,
				KeepAliveConfig: net.KeepAliveConfig{
					Idle: 10 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			// Default value is 15seconds.
			name: "TCPUserTimeout set, but KeepAlive idle not set",
			config: DialerConfig{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Idle: 15 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "KeepAlive idle less than TCPUserTimeout",
			config: DialerConfig{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Idle: 5 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "KeepAlive idle equal to TCPUserTimeout",
			config: DialerConfig{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Idle: 10 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "KeepAlive idle greater than TCPUserTimeout",
			config: DialerConfig{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Idle: 30 * time.Second,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("DialerConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
