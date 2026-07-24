package sessionid

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

type RandomBytesFunc func([]byte) error
type NowMillisFunc func() int64

var (
	uuidStateMu       sync.Mutex
	lastUUIDTimestamp int64 = -1
	uuidSequence      uint32
)

func UUIDv7() string {
	return UUIDv7With(func(bytes []byte) error {
		_, err := rand.Read(bytes)
		return err
	}, func() int64 {
		return time.Now().UnixMilli()
	})
}

func UUIDv7With(randomBytes RandomBytesFunc, nowMillis NowMillisFunc) string {
	random := make([]byte, 16)
	if err := randomBytes(random); err != nil {
		random = make([]byte, 16)
	}
	timestamp := nowMillis()
	uuidStateMu.Lock()
	if timestamp > lastUUIDTimestamp {
		uuidSequence = uint32(random[6])<<24 | uint32(random[7])<<16 | uint32(random[8])<<8 | uint32(random[9])
		lastUUIDTimestamp = timestamp
	} else {
		uuidSequence++
		if uuidSequence == 0 {
			lastUUIDTimestamp++
		}
	}
	resolvedTimestamp := lastUUIDTimestamp
	resolvedSequence := uuidSequence
	uuidStateMu.Unlock()

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
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7],
		bytes[8], bytes[9], bytes[10], bytes[11], bytes[12], bytes[13], bytes[14], bytes[15])
}

func ResetUUIDv7ForTest() {
	uuidStateMu.Lock()
	defer uuidStateMu.Unlock()
	lastUUIDTimestamp = -1
	uuidSequence = 0
}
