// Copyright 2025 Redpanda Data, Inc.

package netutil

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"syscall"
	"time"

	"github.com/redpanda-data/benthos/v4/public/service"
)

// DialerConfigSpec returns the config spec for DialerConfig.
func DialerConfigSpec() *service.ConfigField {
	return service.NewObjectField("tcp",
		service.NewDurationField("connect_timeout").
			Description("Maximum amount of time a dial will wait for a connect to complete. Zero disables.").
			Default("0s"),
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
		Optional().
		Advanced()
}

// DialerConfig contains TCP socket configuration options used to configure
// a net.Dialer.
type DialerConfig struct {
	// Timeout is the maximum amount of time a dial will wait for a connect to
	// complete. If Deadline is also set, it may fail earlier.
	//
	// The default is no timeout.
	Timeout         time.Duration
	KeepAliveConfig net.KeepAliveConfig
	// TCPUserTimeout is only supported on Linux since 2.6.37, on other
	// platforms it's ignored.
	// See: https://www.man7.org/linux/man-pages/man7/tcp.7.html.
	TCPUserTimeout time.Duration
}

// DialerConfigFromParsed creates a DialerConfig from a parsed config.
func DialerConfigFromParsed(pConf *service.ParsedConfig) (DialerConfig, error) {
	var (
		conf DialerConfig
		err  error
	)

	conf.Timeout, err = pConf.FieldDuration("connect_timeout")
	if err != nil {
		return conf, err
	}

	conf.TCPUserTimeout, err = pConf.FieldDuration("tcp_user_timeout")
	if err != nil {
		return conf, err
	}

	if pConf.Contains("keep_alive") {
		pc := pConf.Namespace("keep_alive")

		conf.KeepAliveConfig.Idle, err = pc.FieldDuration("idle")
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

// Validate checks that the configuration is valid.
func (c DialerConfig) Validate() error {
	// KeepAlive MUST be greater than TCP_USER_TIMEOUT per RFC 5482.
	// See: https://www.rfc-editor.org/rfc/rfc5482.html
	if c.TCPUserTimeout > 0 && c.KeepAliveConfig.Idle <= c.TCPUserTimeout {
		return fmt.Errorf("keep_alive.idle (%s) must be greater than tcp_user_timeout (%s)", c.KeepAliveConfig.Idle, c.TCPUserTimeout)
	}
	return nil
}

type controlContextFunc func(ctx context.Context, network, address string, conn syscall.RawConn) error

// DecorateDialer applies DialerConfig to a net.Dialer, configuring keep-alive
// and TCP socket options.
func DecorateDialer(d *net.Dialer, conf DialerConfig) error {
	if err := conf.Validate(); err != nil {
		return err
	}

	d.Timeout = conf.Timeout
	d.KeepAliveConfig = conf.KeepAliveConfig
	d.Timeout = conf.Timeout

	fn := d.ControlContext
	if fn == nil && d.Control != nil {
		fn = func(ctx context.Context, network, address string, conn syscall.RawConn) error {
			return d.Control(network, address, conn)
		}
	}
	d.Control = nil
	d.ControlContext = wrapControlContext(fn, conf)

	return nil
}

// controlFunc returns a function that configures TCP socket options.
func wrapControlContext(inner controlContextFunc, conf DialerConfig) controlContextFunc {
	// We only need to wrap the control function if we have a TCPUserTimeout.
	if !isLinux() || conf.TCPUserTimeout <= 0 {
		return inner
	}
	return func(ctx context.Context, network, address string, conn syscall.RawConn) error {
		if inner != nil {
			if err := inner(ctx, network, address, conn); err != nil {
				return err
			}
		}

		// tcpUserTimeout is a linux specific constant that is used to reference
		// the tcp_user_timeout option.
		const tcpUserTimeout = 18

		var syscallErr error
		if err := conn.Control(func(fd uintptr) {
			syscallErr = setsockoptInt(fd, syscall.IPPROTO_TCP,
				tcpUserTimeout, int(conf.TCPUserTimeout.Milliseconds()))
		}); err != nil {
			return fmt.Errorf("failed to set tcp_user_timeout: %w", err)
		}
		if syscallErr != nil {
			return fmt.Errorf("failed to set tcp_user_timeout: %w", syscallErr)
		}

		return nil
	}
}

func isLinux() bool {
	return runtime.GOOS == "linux"
}
