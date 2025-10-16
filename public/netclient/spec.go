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

package netclient

import "github.com/redpanda-data/benthos/v4/public/service"

// ConfigSpec returns the config spec for TCP options.
func ConfigSpec() *service.ConfigField {
	return service.NewObjectField("tcp",
		service.NewObjectField("keep_alive",
			service.NewDurationField("idle").
				Description("Idle is the time that the connection must be idle before the first keep-alive probe is sent. If zero, a default value of 15 seconds is used. If negative, then keep-alive probes are disabled.").
				Default("15s"),
			service.NewDurationField("interval").
				Description("Interval is the time between keep-alive probes. If zero, a default value of 15 seconds is used.").
				Default("15s"),
			service.NewIntField("count").
				Description("Count is the maximum number of keep-alive probes that can go unanswered before dropping a connection. If zero, a default value of 9 is used").
				Default(9),
		).Description("Custom TCP keep-alive probe configuration.").
			Optional(),
		service.NewDurationField("tcp_user_timeout").
			Description("Linux-specific TCP_USER_TIMEOUT defines how long to wait for acknowledgment of transmitted data on an established connection before killing the connection. This allows more fine grained control on the application level as opposed to the system-wide kernel setting, tcp_retries2. If enabled, keep_alive.idle must be set to a greater value. Set to 0 (default) to disable.").
			Default("0s"),
	).
		Description("TCP socket configuration options").
		Optional()
}

// ParseConfig parses a namespaced TCP config.
func ParseConfig(pConf *service.ParsedConfig) (Config, error) {
	cfg := Config{}
	var err error

	cfg.TCPUserTimeout, err = pConf.FieldDuration("tcp_user_timeout")
	if err != nil {
		return cfg, err
	}

	if pConf.Contains("keep_alive") {
		pc := pConf.Namespace("keep_alive")
		// Each field is optional, so ignoring errors.
		if idle, err := pc.FieldDuration("keep_alive"); err == nil {
			cfg.KeepAliveConfig.Idle = idle
		}
		if interval, err := pc.FieldDuration("interval"); err == nil {
			cfg.KeepAliveConfig.Interval = interval
		}
		if count, err := pc.FieldInt("count"); err == nil {
			cfg.KeepAliveConfig.Count = count
		}
		// If KeepAliveConfig.Idle is a negative number then we assume they want
		// KeepAlives disabled as outlined in the idle description.
		if cfg.KeepAliveConfig.Idle >= 0 {
			cfg.KeepAliveConfig.Enable = true
		} else {
			cfg.KeepAliveConfig.Enable = false
		}
	}
	return cfg, nil
}
