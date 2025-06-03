// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/redpanda-data/benthos/v4/internal/periodic"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func init() {
	service.MustRegisterProcessor("benchmark", benchmarkSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Processor, error) {
			reporter := benchmarkLogReporter{logger: mgr.Logger()}
			return newBenchmarkProcFromParsed(conf, mgr, reporter, time.Now)
		},
	)
}

const (
	bmFieldInterval   = "interval"
	bmFieldCountBytes = "count_bytes"
)

func benchmarkSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Categories("Utility").
		Summary("Logs basic throughput statistics of messages that pass through this processor.").
		Description("Logs messages per second and bytes per second of messages that are processed at a regular interval. A summary of the amount of messages processed over the entire lifetime of the processor will also be printed when the processor shuts down.\n\nThe following metrics are exposed:\n- benchmark_messages_per_second (gauge): The current throughput in messages per second\n- benchmark_messages_total (counter): The total number of messages processed\n- benchmark_bytes_per_second (gauge): The current throughput in bytes per second\n- benchmark_bytes_total (counter): The total number of bytes processed").
		Field(service.NewDurationField(bmFieldInterval).
			Description("How often to emit rolling statistics. If set to 0, only a summary will be logged when the processor shuts down.").
			Default("5s"),
		).
		Field(service.NewBoolField(bmFieldCountBytes).
			Description("Whether or not to measure the number of bytes per second of throughput. Counting the number of bytes requires serializing structured data, which can cause an unnecessary performance hit if serialization is not required elsewhere in the pipeline.").
			Default(true),
		)
}

func newBenchmarkProcFromParsed(conf *service.ParsedConfig, mgr *service.Resources, reporter benchmarkReporter, now func() time.Time) (service.Processor, error) {
	interval, err := conf.FieldDuration(bmFieldInterval)
	if err != nil {
		return nil, err
	}
	countBytes, err := conf.FieldBool(bmFieldCountBytes)
	if err != nil {
		return nil, err
	}

	b := &benchmarkProc{
		startTime:  now(),
		countBytes: countBytes,
		reporter:   reporter,
		now:        now,

		mMsgPerSec:   mgr.Metrics().NewGauge("benchmark_messages_per_second"),
		mTotalMsgs:   mgr.Metrics().NewCounter("benchmark_messages_total"),
		mBytesPerSec: mgr.Metrics().NewGauge("benchmark_bytes_per_second"),
		mTotalBytes:  mgr.Metrics().NewCounter("benchmark_bytes_total"),
	}

	if interval.String() != "0s" {
		b.periodic = periodic.New(interval, func() {
			stats := b.sampleRolling()
			b.printStats("rolling", stats, interval)
		})
		b.periodic.Start()
	}

	return b, nil
}

type benchmarkProc struct {
	startTime  time.Time
	countBytes bool
	reporter   benchmarkReporter

	lock         sync.Mutex
	rollingStats benchmarkStats
	totalStats   benchmarkStats
	closed       bool

	periodic *periodic.Periodic
	now      func() time.Time

	mMsgPerSec   *service.MetricGauge
	mBytesPerSec *service.MetricGauge
	mTotalMsgs   *service.MetricCounter
	mTotalBytes  *service.MetricCounter
}

func (b *benchmarkProc) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	var bytesCount float64
	if b.countBytes {
		bytes, err := msg.AsBytes()
		if err != nil {
			return nil, fmt.Errorf("getting message bytes: %w", err)
		}
		bytesCount = float64(len(bytes))
	}

	b.lock.Lock()
	b.rollingStats.msgCount++
	b.totalStats.msgCount++
	if b.countBytes {
		b.rollingStats.msgBytesCount += bytesCount
		b.totalStats.msgBytesCount += bytesCount
	}
	b.lock.Unlock()

	b.mTotalMsgs.Incr(1)
	b.mTotalBytes.IncrFloat64(bytesCount)

	return service.MessageBatch{msg}, nil
}

func (b *benchmarkProc) Close(ctx context.Context) error {
	// 2024-11-05: We have to guard against Close being from multiple goroutines
	// at the same time.
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.closed {
		return nil
	}
	if b.periodic != nil {
		b.periodic.Stop()
	}
	b.printStats("total", b.totalStats, b.now().Sub(b.startTime))
	b.closed = true
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
	msgsPerSec := stats.msgCount / secs
	b.mMsgPerSec.SetFloat64(msgsPerSec)
	bytesPerSec := stats.msgBytesCount / secs
	b.mBytesPerSec.SetFloat64(bytesPerSec)

	if b.countBytes {
		b.reporter.reportStats(window, msgsPerSec, bytesPerSec)
	} else {
		b.reporter.reportStatsNoBytes(window, msgsPerSec)
	}
}

type benchmarkStats struct {
	msgCount      float64
	msgBytesCount float64
}

type benchmarkReporter interface {
	reportStats(window string, msgsPerSec, bytesPerSec float64)
	reportStatsNoBytes(window string, msgsPerSec float64)
}

type benchmarkLogReporter struct {
	logger *service.Logger
}

func (l benchmarkLogReporter) reportStats(window string, msgsPerSec, bytesPerSec float64) {
	l.logger.
		With(
			"msg/sec", msgsPerSec,
			"bytes/sec", bytesPerSec,
		).
		Infof(
			"%s stats: %s msg/sec, %s/sec",
			window,
			humanize.Ftoa(msgsPerSec),
			humanize.Bytes(uint64(bytesPerSec)),
		)
}

func (l benchmarkLogReporter) reportStatsNoBytes(window string, msgsPerSec float64) {
	l.logger.
		With("msg/sec", msgsPerSec).
		Infof(
			"%s stats: %s msg/sec",
			window,
			humanize.Ftoa(msgsPerSec),
		)
}
