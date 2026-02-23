// Copyright 2025 Redpanda Data, Inc.

package message

import "maps"

// Contains underlying allocated data for messages.
type messageData struct {
	rawBytes []byte // Contents are always read-only
	err      error

	// Mutable when readOnlyStructured = false
	readOnlyStructured bool
	structured         any // Sometimes mutable

	// Mutable when readOnlyMeta = false
	readOnlyMeta bool
	metadata     map[string]metaValue
}

// metaValue stores a metadata entry together with its mutability state.
// For values stored via MetaSetMut, immutable is false and v is the value as
// provided by the caller. For values stored via MetaSetImmut, immutable is
// true and v is the ImmutableValue; callers requesting mutation receive
// v.(immutableMeta).Copy() rather than v itself.
type metaValue struct {
	v        any
	immutable bool
}

// immutableMeta is satisfied by public/service.ImmutableValue. It is defined
// here so internal/message need not import public/service.
type immutableMeta interface {
	Copy() any
}

// defaultMetadataSize specifies how many metadata entries are preallocated by
// default when adding metadata to a message.
const defaultMetadataSize = 5

func newMessageBytes(content []byte) *messageData {
	return &messageData{
		rawBytes: content,
		metadata: nil,
		err:      nil,
	}
}

func (m *messageData) SetBytes(d []byte) {
	m.rawBytes = d
	m.structured = nil
}

func (m *messageData) AsBytes() []byte {
	if len(m.rawBytes) == 0 && m.structured != nil {
		m.rawBytes = encodeJSON(m.structured)
	}
	return m.rawBytes
}

func (m *messageData) HasBytes() bool {
	return m.rawBytes != nil
}

func (m *messageData) HasStructured() bool {
	return m.structured != nil
}

func (m *messageData) SetStructured(jObj any) {
	m.rawBytes = nil
	if jObj == nil {
		m.rawBytes = []byte(`null`)
		m.structured = nil
		m.readOnlyStructured = false
		return
	}
	m.rawBytes = nil
	m.structured = jObj
	m.readOnlyStructured = true
}

func (m *messageData) SetStructuredMut(jObj any) {
	m.SetStructured(jObj)
	m.readOnlyStructured = false
}

func (m *messageData) AsStructured() (any, error) {
	if m.structured != nil {
		return m.structured, nil
	}

	if len(m.rawBytes) == 0 {
		return nil, ErrMessagePartNotExist // TODO: Need this?
	}

	var err error
	m.structured, err = decodeJSON(m.rawBytes)
	return m.structured, err
}

func (m *messageData) AsStructuredMut() (any, error) {
	if m.readOnlyStructured {
		if m.structured != nil {
			m.structured = cloneGeneric(m.structured)
		}
		m.readOnlyStructured = false
	}

	v, err := m.AsStructured()
	if err != nil {
		return nil, err
	}

	// Bytes need resetting as our structured form may change
	m.rawBytes = nil
	return v, nil
}

// ShallowCopy returns a copy of the message data that can be mutated without
// mutating the original message contents (metadata and structured data).
//
// Both the original and the copy share the underlying map and structured
// object. Either can mutate without affecting the other because they both have
// readOnly flags set to true, so any mutation triggers a copy-on-write clone
// first. The original is marked read-only here to prevent it from writing
// directly to the shared map while the copy may be concurrently cloning it.
func (m *messageData) ShallowCopy() *messageData {
	m.readOnlyMeta = true
	m.readOnlyStructured = true
	return &messageData{
		rawBytes: m.rawBytes,
		err:      m.err,

		readOnlyStructured: true,
		structured:         m.structured,

		readOnlyMeta: true,
		metadata:     m.metadata,
	}
}

// DeepCopy returns a copy of the message data that can be mutated without
// mutating the original message contents (metadata and structured data).
// Mutable metadata values are deeply copied via cloneGeneric. Immutable
// metadata values are shared across the copy — they are safe to share because
// MetaGetMut invokes Copy() lazily on demand.
//
// This is worth doing on values persisted outside of the lifetime of a
// transaction unless some other strategy is used for persistence.
func (m *messageData) DeepCopy() *messageData {
	var clonedMeta map[string]metaValue
	if m.metadata != nil {
		clonedMeta = make(map[string]metaValue, len(m.metadata))
		for k, mv := range m.metadata {
			if mv.immutable {
				// Safe to share: Copy() is invoked lazily via MetaGetMut.
				clonedMeta[k] = mv
			} else {
				clonedMeta[k] = metaValue{v: cloneGeneric(mv.v)}
			}
		}
	}

	var bytesCopy []byte
	if len(m.rawBytes) > 0 {
		bytesCopy = make([]byte, len(m.rawBytes))
		copy(bytesCopy, m.rawBytes)
	}

	var structuredCopy any
	if m.structured != nil {
		structuredCopy = cloneGeneric(m.structured)
	}

	return &messageData{
		rawBytes:   bytesCopy,
		err:        m.err,
		structured: structuredCopy,
		metadata:   clonedMeta,
	}
}

func (m *messageData) IsEmpty() bool {
	return len(m.rawBytes) == 0 && m.structured == nil
}

func (m *messageData) writeableMeta() {
	if !m.readOnlyMeta {
		return
	}

	var clonedMeta map[string]metaValue
	if m.metadata != nil {
		clonedMeta = make(map[string]metaValue, len(m.metadata))
		// metaValue entries are copied by struct value; v pointers are shared.
		// Immutable entries remain safe to share (Copy() is lazy via MetaGetMut).
		// Plain reference-type values are shallow-shared at map-container level.
		maps.Copy(clonedMeta, m.metadata)
	}

	m.metadata = clonedMeta
	m.readOnlyMeta = false
}

// MetaGetImmut returns a metadata value if a key exists. The returned value
// must be treated as read-only. For immutable entries the ImmutableValue itself
// is returned; use MetaGetMut to obtain a mutable copy instead.
func (m *messageData) MetaGetImmut(key string) (any, bool) {
	if m.metadata == nil {
		return nil, false
	}
	mv, exists := m.metadata[key]
	return mv.v, exists
}

// MetaGetMut returns a metadata value if a key exists. For entries stored via
// MetaSetImmut, Copy() is called to deliver a fresh mutable instance. Entries
// stored via MetaSetMut are returned as-is.
func (m *messageData) MetaGetMut(key string) (any, bool) {
	if m.metadata == nil {
		return nil, false
	}
	mv, exists := m.metadata[key]
	if !exists {
		return nil, false
	}
	if mv.immutable {
		return mv.v.(immutableMeta).Copy(), true
	}
	return mv.v, true
}

// MetaSetImmut stores value under key, tagged as immutable. Callers retrieving
// via MetaGetMut or MetaIterMut will receive a fresh Copy() of the value.
func (m *messageData) MetaSetImmut(key string, value immutableMeta) {
	m.writeableMeta()
	if m.metadata == nil {
		m.metadata = make(map[string]metaValue, defaultMetadataSize)
	}
	m.metadata[key] = metaValue{v: value, immutable: true}
}

// MetaSetMut stores value under key as a mutable entry. If value is a
// reference type (slice, map, pointer) the caller must not mutate it after
// handing it to MetaSetMut; the entry is stored directly and is shared with
// any shallow copies of this messageData.
func (m *messageData) MetaSetMut(key string, value any) {
	m.writeableMeta()
	if m.metadata == nil {
		m.metadata = make(map[string]metaValue, defaultMetadataSize)
	}
	m.metadata[key] = metaValue{v: value}
}

func (m *messageData) MetaDelete(key string) {
	m.writeableMeta()
	delete(m.metadata, key)
}

// MetaIterMut iterates each metadata key/value pair. For entries stored via
// MetaSetImmut, Copy() is called before yielding so that the caller receives a
// mutable instance. Entries stored via MetaSetMut are yielded as-is.
func (m *messageData) MetaIterMut(f func(k string, v any) error) error {
	for k, mv := range m.metadata {
		v := mv.v
		if mv.immutable {
			v = mv.v.(immutableMeta).Copy()
		}
		if err := f(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (m *messageData) ErrorGet() error {
	return m.err
}

func (m *messageData) ErrorSet(err error) {
	m.err = err
}
