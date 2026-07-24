package gillmprovider

import (
	"crypto/rand"
	"fmt"
	"math"
	"sync"
	"time"
)

type RandomBytesFunc func([]byte) error
type NowMillisFunc func() int64

// UUIDv7Generator owns the monotonic sequence state for one UUID stream.
// Separate application instances can use independent generators without
// sharing mutable package state.
type UUIDv7Generator struct {
	mu            sync.Mutex
	lastTimestamp int64
	sequence      uint32
	randomBytes   RandomBytesFunc
	nowMillis     NowMillisFunc
}

func NewUUIDv7Generator(randomBytes RandomBytesFunc, nowMillis NowMillisFunc) *UUIDv7Generator {
	if randomBytes == nil {
		randomBytes = func(bytes []byte) error {
			_, err := rand.Read(bytes)
			return err
		}
	}
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}
	return &UUIDv7Generator{
		lastTimestamp: math.MinInt64,
		randomBytes:   randomBytes,
		nowMillis:     nowMillis,
	}
}

var defaultUUIDv7Generator = NewUUIDv7Generator(nil, nil)

// UUIDv7 generates an RFC 9562 time-ordered identifier.
func UUIDv7() string {
	return defaultUUIDv7Generator.Generate()
}

// UUIDv7With preserves Gi's injectable compatibility surface while sharing the
// same monotonic stream as UUIDv7.
func UUIDv7With(randomBytes RandomBytesFunc, nowMillis NowMillisFunc) string {
	return defaultUUIDv7Generator.generateWith(randomBytes, nowMillis)
}

func (g *UUIDv7Generator) Generate() string {
	return g.generateWith(g.randomBytes, g.nowMillis)
}

func (g *UUIDv7Generator) generateWith(randomBytes RandomBytesFunc, nowMillis NowMillisFunc) string {
	random := make([]byte, 16)
	if randomBytes == nil || randomBytes(random) != nil {
		clear(random)
	}
	timestamp := time.Now().UnixMilli()
	if nowMillis != nil {
		timestamp = nowMillis()
	}

	g.mu.Lock()
	if timestamp > g.lastTimestamp {
		g.sequence = uint32(random[6])<<24 |
			uint32(random[7])<<16 |
			uint32(random[8])<<8 |
			uint32(random[9])
		g.lastTimestamp = timestamp
	} else {
		g.sequence++
		if g.sequence == 0 {
			g.lastTimestamp++
		}
	}
	resolvedTimestamp := g.lastTimestamp
	resolvedSequence := g.sequence
	g.mu.Unlock()

	bytes := make([]byte, 16)
	bytes[0] = byte(resolvedTimestamp / 0x10000000000)
	bytes[1] = byte(resolvedTimestamp / 0x100000000)
	bytes[2] = byte(resolvedTimestamp / 0x1000000)
	bytes[3] = byte(resolvedTimestamp / 0x10000)
	bytes[4] = byte(resolvedTimestamp / 0x100)
	bytes[5] = byte(resolvedTimestamp)
	bytes[6] = 0x70 | byte((resolvedSequence>>28)&0x0f)
	bytes[7] = byte(resolvedSequence >> 20)
	bytes[8] = 0x80 | byte((resolvedSequence>>14)&0x3f)
	bytes[9] = byte(resolvedSequence >> 6)
	bytes[10] = byte((resolvedSequence&0x3f)<<2) | (random[10] & 0x03)
	copy(bytes[11:], random[11:16])

	return fmt.Sprintf(
		"%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7],
		bytes[8], bytes[9], bytes[10], bytes[11], bytes[12], bytes[13], bytes[14], bytes[15],
	)
}

// ResetUUIDv7ForTest resets only the package default generator.
func ResetUUIDv7ForTest() {
	defaultUUIDv7Generator.mu.Lock()
	defer defaultUUIDv7Generator.mu.Unlock()
	defaultUUIDv7Generator.lastTimestamp = math.MinInt64
	defaultUUIDv7Generator.sequence = 0
}
