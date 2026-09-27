// Copyright 2025 Redpanda Data, Inc.

package retries

import (
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/redpanda-data/benthos/v4/public/service"
)

const (
	crboFieldMaxRetries     = "max_retries"
	crboFieldBackOff        = "backoff"
	crboFieldInitInterval   = "initial_interval"
	crboFieldMaxInterval    = "max_interval"
	crboFieldMaxElapsedTime = "max_elapsed_time"
)

func fieldDurationOrEmptyStr(pConf *service.ParsedConfig, path ...string) (time.Duration, error) {
	if dStr, err := pConf.FieldString(path...); err == nil && dStr == "" {
		return 0, nil
	}
	return pConf.FieldDuration(path...)
}

// CommonRetryBackOffCtorFromParsed extracts the retry backoff fields from the config and returns a retry backoff constructor.
func CommonRetryBackOffCtorFromParsed(pConf *service.ParsedConfig) (ctor func() backoff.BackOff, err error) {
	var maxRetries int
	if maxRetries, err = pConf.FieldInt(crboFieldMaxRetries); err != nil {
		return
	}

	var initInterval, maxInterval, maxElapsed time.Duration
	if pConf.Contains(crboFieldBackOff) {
		bConf := pConf.Namespace(crboFieldBackOff)
		if initInterval, err = fieldDurationOrEmptyStr(bConf, crboFieldInitInterval); err != nil {
			return
		}
		if maxInterval, err = fieldDurationOrEmptyStr(bConf, crboFieldMaxInterval); err != nil {
			return
		}
		if maxElapsed, err = fieldDurationOrEmptyStr(bConf, crboFieldMaxElapsedTime); err != nil {
			return
		}
	}

	return func() backoff.BackOff {
		boff := backoff.NewExponentialBackOff()

		boff.InitialInterval = initInterval
		boff.MaxInterval = maxInterval
		boff.MaxElapsedTime = maxElapsed

		if maxRetries > 0 {
			return backoff.WithMaxRetries(boff, uint64(maxRetries))
		}
		return boff
	}, nil
}
