// Copyright 2025 Redpanda Data, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package netutil

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
					Idle: 10 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			// Default value is 15seconds.
			name: "TCPUserTimeout set, but KeepAlive idle not set",
			config: Config{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Idle: 15 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "KeepAlive idle less than TCPUserTimeout",
			config: Config{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Idle: 5 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "KeepAlive idle equal to TCPUserTimeout",
			config: Config{
				TCPUserTimeout: 10 * time.Second,
				KeepAliveConfig: net.KeepAliveConfig{
					Idle: 10 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "KeepAlive idle greater than TCPUserTimeout",
			config: Config{
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
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
