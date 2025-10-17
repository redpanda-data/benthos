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

package netutilspec

import (
	"github.com/redpanda-data/benthos/v4/public/service"
	"github.com/redpanda-data/benthos/v4/public/utils/netutil"
)

// DialerConfigSpec returns the config spec for DialerConfig.
func DialerConfigSpec() *service.ConfigField {
	return service.NewObjectField("tcp",
		service.NewObjectField("keep_alive",
			service.NewDurationField("idle").
				Description("Duration the connection must be idle before sending the first keep-alive probe. "+
					"Zero defaults to 15s. Negative values disable keep-alive probes.").
				Default("15s"),
			service.NewDurationField("interval").
				Description("Duration between keep-alive probes. Zero defaults to 15s.").
				Default("15s"),
			service.NewIntField("count").
				Description("Maximum unanswered keep-alive probes before dropping the connection. Zero defaults to 9.").
				Default(9),
		).Description("TCP keep-alive probe configuration.").
			Optional(),
		service.NewDurationField("tcp_user_timeout").
			Description("Maximum time to wait for acknowledgment of transmitted data before killing the connection. "+
				"Linux-only (kernel 2.6.37+), ignored on other platforms. "+
				"When enabled, keep_alive.idle must be greater than this value per RFC 5482. Zero disables.").
			Default("0s"),
	).
		Description("TCP socket configuration.").
		Optional()
}

// DialerConfigFromParsed creates a DialerConfig from a parsed config.
func DialerConfigFromParsed(pConf *service.ParsedConfig) (netutil.DialerConfig, error) {
	var (
		conf netutil.DialerConfig
		err  error
	)

	conf.TCPUserTimeout, err = pConf.FieldDuration("tcp_user_timeout")
	if err != nil {
		return conf, err
	}

	if pConf.Contains("keep_alive") {
		pc := pConf.Namespace("keep_alive")

		conf.KeepAliveConfig.Idle, err = pc.FieldDuration("keep_alive")
		if err != nil {
			return conf, err
		}

		conf.KeepAliveConfig.Interval, err = pc.FieldDuration("interval")
		if err != nil {
			return conf, err
		}

		conf.KeepAliveConfig.Count, err = pc.FieldInt("count")
		if err != nil {
			return conf, err
		}

		// If KeepAliveConfig.Idle is a negative number then we assume they want
		// KeepAlives disabled as outlined in the idle description.
		if conf.KeepAliveConfig.Idle >= 0 {
			conf.KeepAliveConfig.Enable = true
		} else {
			conf.KeepAliveConfig.Enable = false
		}
	}

	return conf, nil
}
