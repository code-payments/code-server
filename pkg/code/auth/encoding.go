package auth

import (
	"math"
	"sort"
	"sync"
	"unicode/utf8"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoiface"
)

// Primarily based off of https://github.com/protocolbuffers/protobuf-go/blob/v1.28.1/proto/encode.go
// but tweaked for consistent marshalling using field number ordering.
//
// todo: Potentially use a custom approach. This is a temporary measure to fix
//       discrepancies between Go and other language implementations that result
//       in signature mismatch issues due to not using field numbers for ordering.

const speculativeLength = 1

var wireTypes = map[protoreflect.Kind]protowire.Type{
	protoreflect.BoolKind:     protowire.VarintType,
	protoreflect.EnumKind:     protowire.VarintType,
	protoreflect.Int32Kind:    protowire.VarintType,
	protoreflect.Sint32Kind:   protowire.VarintType,
	protoreflect.Uint32Kind:   protowire.VarintType,
	protoreflect.Int64Kind:    protowire.VarintType,
	protoreflect.Sint64Kind:   protowire.VarintType,
	protoreflect.Uint64Kind:   protowire.VarintType,
	protoreflect.Sfixed32Kind: protowire.Fixed32Type,
	protoreflect.Fixed32Kind:  protowire.Fixed32Type,
	protoreflect.FloatKind:    protowire.Fixed32Type,
	protoreflect.Sfixed64Kind: protowire.Fixed64Type,
	protoreflect.Fixed64Kind:  protowire.Fixed64Type,
	protoreflect.DoubleKind:   protowire.Fixed64Type,
	protoreflect.StringKind:   protowire.BytesType,
	protoreflect.BytesKind:    protowire.BytesType,
	protoreflect.MessageKind:  protowire.BytesType,
	protoreflect.GroupKind:    protowire.StartGroupType,
}

func forceConsistentMarshal(m proto.Message) ([]byte, error) {
	out, err := consistentMarshal(nil, m.ProtoReflect())
	if err != nil {
		return nil, err
	}
	return out.Buf, nil
}

func consistentMarshal(b []byte, m protoreflect.Message) (out protoiface.MarshalOutput, err error) {
	out.Buf, err = consistentMarshalMessageSlow(b, m)
	if err != nil {
		return out, err
	}
	return out, checkInitialized(m)
}

func consistentMarshalMessageSlow(b []byte, m protoreflect.Message) ([]byte, error) {
	var err error
	rangeFields(m, numberFieldOrder, func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		b, err = marshalField(b, fd, v)
		return err == nil
	})
	if err != nil {
		return b, err
	}
	b = append(b, m.GetUnknown()...)
	return b, nil
}

func marshalSingular(b []byte, fd protoreflect.FieldDescriptor, v protoreflect.Value) ([]byte, error) {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		b = protowire.AppendVarint(b, protowire.EncodeBool(v.Bool()))
	case protoreflect.EnumKind:
		b = protowire.AppendVarint(b, uint64(v.Enum()))
	case protoreflect.Int32Kind:
		b = protowire.AppendVarint(b, uint64(int32(v.Int())))
	case protoreflect.Sint32Kind:
		b = protowire.AppendVarint(b, protowire.EncodeZigZag(int64(int32(v.Int()))))
	case protoreflect.Uint32Kind:
		b = protowire.AppendVarint(b, uint64(uint32(v.Uint())))
	case protoreflect.Int64Kind:
		b = protowire.AppendVarint(b, uint64(v.Int()))
	case protoreflect.Sint64Kind:
		b = protowire.AppendVarint(b, protowire.EncodeZigZag(v.Int()))
	case protoreflect.Uint64Kind:
		b = protowire.AppendVarint(b, v.Uint())
	case protoreflect.Sfixed32Kind:
		b = protowire.AppendFixed32(b, uint32(v.Int()))
	case protoreflect.Fixed32Kind:
		b = protowire.AppendFixed32(b, uint32(v.Uint()))
	case protoreflect.FloatKind:
		b = protowire.AppendFixed32(b, math.Float32bits(float32(v.Float())))
	case protoreflect.Sfixed64Kind:
		b = protowire.AppendFixed64(b, uint64(v.Int()))
	case protoreflect.Fixed64Kind:
		b = protowire.AppendFixed64(b, v.Uint())
	case protoreflect.DoubleKind:
		b = protowire.AppendFixed64(b, math.Float64bits(v.Float()))
	case protoreflect.StringKind:
		if enforceUTF8(fd) && !utf8.ValidString(v.String()) {
			return b, errors.Errorf("field %v contains invalid UTF-8", string(fd.FullName()))
		}
		b = protowire.AppendString(b, v.String())
	case protoreflect.BytesKind:
		b = protowire.AppendBytes(b, v.Bytes())
	case protoreflect.MessageKind:
		var pos int
		var err error
		b, pos = appendSpeculativeLength(b)
		b, err = marshalMessage(b, v.Message())
		if err != nil {
			return b, err
		}
		b = finishSpeculativeLength(b, pos)
	case protoreflect.GroupKind:
		var err error
		b, err = marshalMessage(b, v.Message())
		if err != nil {
			return b, err
		}
		b = protowire.AppendVarint(b, protowire.EncodeTag(fd.Number(), protowire.EndGroupType))
	default:
		return b, errors.Errorf("invalid kind %v", fd.Kind())
	}
	return b, nil
}

func marshalMessage(b []byte, m protoreflect.Message) ([]byte, error) {
	out, err := consistentMarshal(b, m)
	return out.Buf, err
}

func marshalField(b []byte, fd protoreflect.FieldDescriptor, value protoreflect.Value) ([]byte, error) {
	switch {
	case fd.IsList():
		return marshalList(b, fd, value.List())
	case fd.IsMap():
		return marshalMap(b, fd, value.Map())
	default:
		b = protowire.AppendTag(b, fd.Number(), wireTypes[fd.Kind()])
		return marshalSingular(b, fd, value)
	}
}

func marshalList(b []byte, fd protoreflect.FieldDescriptor, list protoreflect.List) ([]byte, error) {
	if fd.IsPacked() && list.Len() > 0 {
		b = protowire.AppendTag(b, fd.Number(), protowire.BytesType)
		b, pos := appendSpeculativeLength(b)
		for i, llen := 0, list.Len(); i < llen; i++ {
			var err error
			b, err = marshalSingular(b, fd, list.Get(i))
			if err != nil {
				return b, err
			}
		}
		b = finishSpeculativeLength(b, pos)
		return b, nil
	}

	kind := fd.Kind()
	for i, llen := 0, list.Len(); i < llen; i++ {
		var err error
		b = protowire.AppendTag(b, fd.Number(), wireTypes[kind])
		b, err = marshalSingular(b, fd, list.Get(i))
		if err != nil {
			return b, err
		}
	}
	return b, nil
}

func marshalMap(b []byte, fd protoreflect.FieldDescriptor, mapv protoreflect.Map) ([]byte, error) {
	keyf := fd.MapKey()
	valf := fd.MapValue()
	var err error
	rangeEntries(mapv, genericKeyOrder, func(key protoreflect.MapKey, value protoreflect.Value) bool {
		b = protowire.AppendTag(b, fd.Number(), protowire.BytesType)
		var pos int
		b, pos = appendSpeculativeLength(b)

		b, err = marshalField(b, keyf, key.Value())
		if err != nil {
			return false
		}
		b, err = marshalField(b, valf, value)
		if err != nil {
			return false
		}
		b = finishSpeculativeLength(b, pos)
		return true
	})
	return b, err
}

// fieldOrder specifies the ordering to visit message fields.
// It is a function that reports whether x is ordered before y.
type fieldOrder func(x, y protoreflect.FieldDescriptor) bool

var (
	// numberFieldOrder sorts fields by their field number.
	numberFieldOrder fieldOrder = func(x, y protoreflect.FieldDescriptor) bool {
		return x.Number() < y.Number()
	}
)

type messageField struct {
	fd protoreflect.FieldDescriptor
	v  protoreflect.Value
}

var messageFieldPool = sync.Pool{
	New: func() interface{} { return new([]messageField) },
}

type (
	// fieldRnger is an interface for visiting all fields in a message.
	// The protoreflect.Message type implements this interface.
	fieldRanger interface{ Range(visitField) }
	// visitField is called every time a message field is visited.
	visitField = func(protoreflect.FieldDescriptor, protoreflect.Value) bool
)

func rangeFields(fs fieldRanger, less fieldOrder, fn visitField) {
	if less == nil {
		fs.Range(fn)
		return
	}

	// Obtain a pre-allocated scratch buffer.
	p := messageFieldPool.Get().(*[]messageField)
	fields := (*p)[:0]
	defer func() {
		if cap(fields) < 1024 {
			*p = fields
			messageFieldPool.Put(p)
		}
	}()

	// Collect all fields in the message and sort them.
	fs.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		fields = append(fields, messageField{fd, v})
		return true
	})
	sort.Slice(fields, func(i, j int) bool {
		return less(fields[i].fd, fields[j].fd)
	})

	// Visit the fields in the specified ordering.
	for _, f := range fields {
		if !fn(f.fd, f.v) {
			return
		}
	}
}

// keyOrder specifies the ordering to visit map entries.
// It is a function that reports whether x is ordered before y.
type keyOrder func(x, y protoreflect.MapKey) bool

var (
	// genericKeyOrder sorts false before true, numeric keys in ascending order,
	// and strings in lexicographical ordering according to UTF-8 codepoints.
	genericKeyOrder keyOrder = func(x, y protoreflect.MapKey) bool {
		switch x.Interface().(type) {
		case bool:
			return !x.Bool() && y.Bool()
		case int32, int64:
			return x.Int() < y.Int()
		case uint32, uint64:
			return x.Uint() < y.Uint()
		case string:
			return x.String() < y.String()
		default:
			panic("invalid map key type")
		}
	}
)

type (
	// entryRanger is an interface for visiting all fields in a message.
	// The protoreflect.Map type implements this interface.
	entryRanger interface{ Range(visitEntry) }
	// visitEntry is called every time a map entry is visited.
	visitEntry = func(protoreflect.MapKey, protoreflect.Value) bool
)

type mapEntry struct {
	k protoreflect.MapKey
	v protoreflect.Value
}

var mapEntryPool = sync.Pool{
	New: func() interface{} { return new([]mapEntry) },
}

// rangeEntries iterates over the entries of es according to the specified order.
func rangeEntries(es entryRanger, less keyOrder, fn visitEntry) {
	if less == nil {
		es.Range(fn)
		return
	}

	// Obtain a pre-allocated scratch buffer.
	p := mapEntryPool.Get().(*[]mapEntry)
	entries := (*p)[:0]
	defer func() {
		if cap(entries) < 1024 {
			*p = entries
			mapEntryPool.Put(p)
		}
	}()

	// Collect all entries in the map and sort them.
	es.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
		entries = append(entries, mapEntry{k, v})
		return true
	})
	sort.Slice(entries, func(i, j int) bool {
		return less(entries[i].k, entries[j].k)
	})

	// Visit the entries in the specified ordering.
	for _, e := range entries {
		if !fn(e.k, e.v) {
			return
		}
	}
}

func checkInitialized(m protoreflect.Message) error {
	return checkInitializedSlow(m)
}

func checkInitializedSlow(m protoreflect.Message) error {
	md := m.Descriptor()
	fds := md.Fields()
	for i, nums := 0, md.RequiredNumbers(); i < nums.Len(); i++ {
		fd := fds.ByNumber(nums.Get(i))
		if !m.Has(fd) {
			return errors.Errorf("required field %v not set", string(fd.FullName()))
		}
	}
	var err error
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList():
			if fd.Message() == nil {
				return true
			}
			for i, list := 0, v.List(); i < list.Len() && err == nil; i++ {
				err = checkInitialized(list.Get(i).Message())
			}
		case fd.IsMap():
			if fd.MapValue().Message() == nil {
				return true
			}
			v.Map().Range(func(key protoreflect.MapKey, v protoreflect.Value) bool {
				err = checkInitialized(v.Message())
				return err == nil
			})
		default:
			if fd.Message() == nil {
				return true
			}
			err = checkInitialized(v.Message())
		}
		return err == nil
	})
	return err
}

func enforceUTF8(fd protoreflect.FieldDescriptor) bool {
	return fd.Syntax() == protoreflect.Proto3
}

func appendSpeculativeLength(b []byte) ([]byte, int) {
	pos := len(b)
	b = append(b, "\x00\x00\x00\x00"[:speculativeLength]...)
	return b, pos
}

func finishSpeculativeLength(b []byte, pos int) []byte {
	mlen := len(b) - pos - speculativeLength
	msiz := protowire.SizeVarint(uint64(mlen))
	if msiz != speculativeLength {
		for i := 0; i < msiz-speculativeLength; i++ {
			b = append(b, 0)
		}
		copy(b[pos+msiz:], b[pos+speculativeLength:])
		b = b[:pos+msiz+mlen]
	}
	protowire.AppendVarint(b[:pos], uint64(mlen))
	return b
}
