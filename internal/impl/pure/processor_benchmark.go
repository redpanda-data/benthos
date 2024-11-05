// Copyright 2024 Redpanda Data, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package pure

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func init() {
	err := service.RegisterProcessor("benchmark", benchmarkSpec(), newBenchmarkProcFromConfig)
	if err != nil {
		panic(err)
	}
}

const (
	bmFieldInterval = "interval"
)

func benchmarkSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Categories("Utility").
		Summary("Logs basic throughput statistics of message that pass through this processor.").
		Description("Logs messages per second and bytes per second of messages that are processed at a regular interval. A summary of the amount of messages processed over the entire lifetime of the processor will also be printed when the processor shuts down.").
		Field(service.NewDurationField(bmFieldInterval).
			Description("How often to emit rolling statistics. If set to 0, only a summary will be logged when the processor shuts down.").
			Default("5s").
			Description("How often to emit rolling statistics."),
		)
}

func newBenchmarkProcFromConfig(conf *service.ParsedConfig, mgr *service.Resources) (service.Processor, error) {
	interval, err := conf.FieldDuration(bmFieldInterval)
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})
	b := &benchmarkProc{
		startTime:       time.Now(),
		rollingInterval: interval,
		logger:          mgr.Logger(),
		done:            done,
	}

	if interval.String() != "0s" {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			defer b.wg.Done()

			for {
				select {
				case <-done:
					break

				case <-ticker.C:
					stats := b.sampleRolling()
					b.printStats("rolling", stats, b.rollingInterval)
				}
			}
		}()
	}

	return b, nil
}

type benchmarkProc struct {
	startTime       time.Time
	rollingInterval time.Duration
	logger          *service.Logger

	lock         sync.Mutex
	rollingStats benchmarkStats
	totalStats   benchmarkStats

	wg   sync.WaitGroup
	done chan<- struct{}
}

func (b *benchmarkProc) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	bytes, err := msg.AsBytes()
	if err != nil {
		return nil, fmt.Errorf("getting message bytes: %w", err)
	}

	bytesCount := float64(len(bytes))

	b.lock.Lock()
	b.rollingStats.recordMessage(bytesCount)
	b.totalStats.recordMessage(bytesCount)
	b.lock.Unlock()

	return service.MessageBatch{msg}, nil
}

func (b *benchmarkProc) Close(ctx context.Context) error {
	if b.done == nil {
		return nil
	}

	close(b.done)
	b.wg.Wait()
	b.done = nil

	b.printStats("total", b.totalStats, time.Since(b.startTime))
	return nil
}

func (b *benchmarkProc) sampleRolling() benchmarkStats {
	b.lock.Lock()
	defer b.lock.Unlock()

	s := b.rollingStats
	b.rollingStats.msgCount = 0
	b.rollingStats.msgBytesCount = 0
	return s
}

func (b *benchmarkProc) printStats(window string, stats benchmarkStats, interval time.Duration) {
	secs := interval.Seconds()
	b.logger.Infof(
		"%s stats: %s msg/sec, %s/sec",
		window,
		humanize.Ftoa(stats.msgCount/secs),
		humanize.Bytes(uint64(stats.msgBytesCount/secs)),
	)
}

type benchmarkStats struct {
	msgCount      float64
	msgBytesCount float64
}

func (s *benchmarkStats) recordMessage(bytesCount float64) {
	s.msgCount++
	s.msgCount++
	s.msgBytesCount += float64(bytesCount)
}
