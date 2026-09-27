// Copyright 2026 Redpanda Data, Inc.

package service

import (
	"github.com/cenkalti/backoff/v4"
)

// NewMaxRetriesField defines a new advanced integer config field named
// `max_retries` that limits the number of retry attempts, where zero means
// there is no limit.
func NewMaxRetriesField(defaultMaxRetries int) *ConfigField {
	return NewIntField("max_retries").
		Description("The maximum number of retries before giving up on the request. If set to zero there is no discrete limit.").
		Default(defaultMaxRetries).
		Advanced()
}

// NewRetryBackOffFields defines the fields `max_retries`, from
// NewMaxRetriesField, and `backoff`, an advanced object with the duration
// fields `initial_interval`, `max_interval` and `max_elapsed_time`. Together
// they configure an exponential backoff between retry attempts that stops
// after `max_retries` attempts or `max_elapsed_time`, where a zero value of
// either means no limit.
//
// The defaults struct is optional, and if provided sets the default values of
// the duration fields. Otherwise the defaults are those of NewBackOffField.
func NewRetryBackOffFields(defaultMaxRetries int, defaults *backoff.ExponentialBackOff) []*ConfigField {
	var (
		initDefault       = "500ms"
		maxDefault        = "10s"
		maxElapsedDefault = "1m"
	)
	if defaults != nil {
		initDefault = defaults.InitialInterval.String()
		maxDefault = defaults.MaxInterval.String()
		maxElapsedDefault = defaults.MaxElapsedTime.String()
	}

	return []*ConfigField{
		NewMaxRetriesField(defaultMaxRetries),
		NewObjectField("backoff",
			NewDurationField("initial_interval").
				Description("The initial period to wait between retry attempts. The retry interval increases for each failed attempt, up to the `backoff.max_interval` value. This field accepts Go duration format strings such as `100ms`, `1s`, or `5s`.").
				Default(initDefault),
			NewDurationField("max_interval").
				Description("The maximum period to wait between retry attempts.").
				Default(maxDefault),
			NewDurationField("max_elapsed_time").
				Description("The maximum period to wait before retry attempts are abandoned. If zero then no limit is used.").
				Default(maxElapsedDefault),
		).
			Description("The exponential backoff between retry attempts. The wait starts at `initial_interval` and grows with each failed attempt, up to `max_interval`.").
			Advanced(),
	}
}
