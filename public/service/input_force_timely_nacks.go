// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ForceTimelyNacksBatched wraps an input implementation with a timely ack
// mechanism only if a field defined by NewTimelyNacksField has been specified
// and is a duration greater than zero.
func ForceTimelyNacksBatched(c *ParsedConfig, i BatchInput) (BatchInput, error) {
	if !c.Contains(ForceTimelyNacksFieldName) {
		return i, nil
	}

	d, err := c.FieldDuration(ForceTimelyNacksFieldName)
	if err != nil {
		return nil, err
	}
	if d <= 0 {
		return i, nil
	}

	return &forceTimelyNacksInputBatched{
		logger:          c.Resources().Logger(),
		maxWaitDuration: d,
		child:           i,
	}, nil
}

var errForceTimelyNacks = errors.New("message acknowledgement exceeded maximum wait and has been rejected")

type forceTimelyNacksInputBatched struct {
	logger          *Logger
	maxWaitDuration time.Duration
	child           BatchInput
}

func (i *forceTimelyNacksInputBatched) Connect(ctx context.Context) error {
	return i.child.Connect(ctx)
}

func (i *forceTimelyNacksInputBatched) ReadBatch(ctx context.Context) (MessageBatch, AckFunc, error) {
	batch, ackFn, err := i.child.ReadBatch(ctx)
	if err != nil {
		return nil, nil, err
	}

	var ackOnce, closeAckedChanOnce sync.Once
	ackedChan := make(chan struct{})

	go func() {
		select {
		case <-ackedChan:
			return
		case <-time.After(i.maxWaitDuration):
			ackOnce.Do(func() {
				i.logger.With("duration", i.maxWaitDuration.String()).Warn("Message acknowledgement exceeded our configured maximum wait period and is being rejected as a result")
				_ = ackFn(context.Background(), errForceTimelyNacks)
			})
		}
	}()

	return batch, func(ctx context.Context, err error) (ackErr error) {
		closeAckedChanOnce.Do(func() {
			close(ackedChan)
		})
		ackOnce.Do(func() {
			ackErr = ackFn(ctx, err)
		})
		return
	}, nil
}

func (i *forceTimelyNacksInputBatched) Close(ctx context.Context) error {
	return i.child.Close(ctx)
}
