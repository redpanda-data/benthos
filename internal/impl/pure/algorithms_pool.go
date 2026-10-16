// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"bytes"
	"sync"
	"sync/atomic"
)

// maxPooledBufferSize caps the scratch buffer a pooled codec keeps between
// calls, so that one unusually large message does not pin memory in the pool.
const maxPooledBufferSize = 256 * 1024

const (
	levelPoolsMin = -8
	levelPoolsMax = 23
)

// levelPools holds one sync.Pool per compression level. Writers such as
// gzip.Writer keep their level across Reset, so they must never be shared
// between levels. Common levels use a fixed array because boxing a negative
// int (e.g. the default level -1) into a sync.Map key allocates.
type levelPools struct {
	small [levelPoolsMax - levelPoolsMin + 1]atomic.Pointer[sync.Pool]
	other sync.Map // int -> *sync.Pool
}

func (l *levelPools) load(level int) *sync.Pool {
	if level >= levelPoolsMin && level <= levelPoolsMax {
		return l.small[level-levelPoolsMin].Load()
	}
	if v, ok := l.other.Load(level); ok {
		return v.(*sync.Pool)
	}
	return nil
}

func (l *levelPools) loadOrCreate(level int) *sync.Pool {
	if level >= levelPoolsMin && level <= levelPoolsMax {
		slot := &l.small[level-levelPoolsMin]
		slot.CompareAndSwap(nil, &sync.Pool{})
		return slot.Load()
	}
	v, _ := l.other.LoadOrStore(level, &sync.Pool{})
	return v.(*sync.Pool)
}

type pooledCompressor struct {
	w ResettableWriter
	// buf is the writer's sink. It lives inside the struct so that dropping
	// an oversized backing array never leaves the writer pointing at it.
	buf bytes.Buffer
}

func (p *pooledCompressor) trimBuffer() {
	if p.buf.Cap() > maxPooledBufferSize {
		p.buf = bytes.Buffer{}
	}
}

type pooledCompress struct {
	ctor  ResettableCompressWriter
	pools levelPools
}

// newPooledCompressFunc returns a CompressFunc that reuses writers created by
// ctor. A writer is only returned to its pool after a fully successful
// compression, and the returned bytes are always a fresh copy that does not
// alias pooled memory.
func newPooledCompressFunc(ctor ResettableCompressWriter) CompressFunc {
	return (&pooledCompress{ctor: ctor}).compress
}

func (p *pooledCompress) compress(level int, b []byte) ([]byte, error) {
	pool := p.pools.load(level)

	var pc *pooledCompressor
	if pool != nil {
		pc, _ = pool.Get().(*pooledCompressor)
	}
	if pc != nil {
		pc.buf.Reset()
		pc.w.Reset(&pc.buf)
	} else {
		pc = &pooledCompressor{}
		w, err := p.ctor(level, &pc.buf)
		if err != nil {
			return nil, err
		}
		pc.w = w
		if pool == nil {
			// Only create a pool once the level has proven valid, so invalid
			// levels cannot grow the set of pools.
			pool = p.pools.loadOrCreate(level)
		}
	}

	_, werr := pc.w.Write(b)
	if cerr := pc.w.Close(); cerr != nil {
		return nil, cerr
	}
	if werr != nil {
		return nil, werr
	}

	out := bytes.Clone(pc.buf.Bytes())
	pc.trimBuffer()
	pool.Put(pc)
	return out, nil
}
