// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/redpanda-data/benthos/v4/internal/component/interop"
	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func init() {
	service.MustRegisterBatchProcessor("bloblang_v2_file", bloblangV2FileProcConfig(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchProcessor, error) {
			return newBloblangV2FileFromParsed(conf, mgr)
		})
}

func bloblangV2FileProcConfig() *service.ConfigSpec {
	return service.NewConfigSpec().
		Categories("Mapping", "Parsing").
		Field(service.NewStringField("").
			Description("Path to a Bloblang V2 mapping file. The file is read once at processor construction; subsequent file changes are picked up only when the config is reloaded.")).
		Summary("Executes a Bloblang V2 mapping loaded from a file on disk.").
		Description(`
Counterpart to the inline `+"`bloblang_v2`"+` processor for cases where the
mapping lives in its own file. The file is loaded and compiled once when the
processor is constructed; the resulting executor is reused for every message,
so there is no per-message file-system overhead.

This is the V2 equivalent of writing `+"`bloblang: 'from \"path\"'`"+` against
the V1 processor. The migrator rewrites such configs to this processor when
upgrading to V2.

Paths are resolved through the host filesystem (typically relative to the
working directory the process started in).

== Imports

The file is parsed as a self-contained V2 mapping. V2 `+"`import`"+`
statements within the file are not currently resolved by this processor —
mappings that need imports should keep them inline via `+"`bloblang_v2`"+`
or wait for follow-up support here.
`).
		Example("File-backed mapping", `
Given a mapping file `+"`./mappings/extract.blobl`"+` containing:

`+"```"+`
output.id = input.id
output.fans = input.fans.filter(f -> f.obsession > 0.5)
`+"```"+`

The pipeline references it by path:`,
			`
pipeline:
  processors:
    - bloblang_v2_file: ./mappings/extract.blobl
`)
}

func newBloblangV2FileFromParsed(conf *service.ParsedConfig, mgr *service.Resources) (*bloblangV2FileProc, error) {
	path, err := conf.FieldString()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errors.New("bloblang_v2_file: path is required")
	}

	f, err := mgr.FS().Open(path)
	if err != nil {
		return nil, fmt.Errorf("bloblang_v2_file: opening %q: %w", path, err)
	}
	defer f.Close()

	srcBytes, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("bloblang_v2_file: reading %q: %w", path, err)
	}

	exec, err := interop.UnwrapManagement(mgr).BloblV2Environment().Parse(string(srcBytes))
	if err != nil {
		return nil, fmt.Errorf("bloblang_v2_file: parsing %q: %w", path, err)
	}

	return &bloblangV2FileProc{exec: exec, log: mgr.Logger(), path: path}, nil
}

type bloblangV2FileProc struct {
	exec *bloblangv2.Executor
	log  *service.Logger
	path string
}

func (p *bloblangV2FileProc) ProcessBatch(_ context.Context, batch service.MessageBatch) ([]service.MessageBatch, error) {
	newBatch := make(service.MessageBatch, 0, len(batch))
	batchSize := len(batch)
	for i, msg := range batch {
		input, err := messageInputValue(msg)
		if err != nil {
			newMsg := msg.Copy()
			newMsg.SetError(err)
			p.log.Errorf("%v", err)
			newBatch = append(newBatch, newMsg)
			continue
		}

		inputMeta := map[string]any{}
		_ = msg.MetaWalkMut(func(k string, v any) error {
			inputMeta[k] = v
			return nil
		})

		ctx := &messageContext{
			msg:        msg,
			input:      input,
			meta:       inputMeta,
			batchIndex: i,
			batchSize:  batchSize,
		}
		output, outputMeta, err := p.exec.QueryMessage(ctx)
		if err != nil {
			if errors.Is(err, bloblangv2.ErrRootDeleted) {
				continue
			}
			newMsg := msg.Copy()
			newMsg.SetError(err)
			p.log.Errorf("%v", err)
			newBatch = append(newBatch, newMsg)
			continue
		}

		newMsg := msg.Copy()
		switch v := output.(type) {
		case []byte:
			newMsg.SetBytes(v)
		case string:
			newMsg.SetBytes([]byte(v))
		default:
			newMsg.SetStructured(output)
		}

		_ = newMsg.MetaWalkMut(func(k string, _ any) error {
			newMsg.MetaDelete(k)
			return nil
		})
		for k, v := range outputMeta {
			newMsg.MetaSetMut(k, v)
		}

		newBatch = append(newBatch, newMsg)
	}
	if len(newBatch) == 0 {
		return nil, nil
	}
	return []service.MessageBatch{newBatch}, nil
}

func (p *bloblangV2FileProc) Close(context.Context) error {
	return nil
}
