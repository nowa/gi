package harness

import "github.com/nowa/gi/gi-agent-core/harness/sessionid"

type RandomBytesFunc = sessionid.RandomBytesFunc
type NowMillisFunc = sessionid.NowMillisFunc

func UUIDv7() string {
	return sessionid.UUIDv7()
}

func UUIDv7With(randomBytes RandomBytesFunc, nowMillis NowMillisFunc) string {
	return sessionid.UUIDv7With(randomBytes, nowMillis)
}

func ResetUUIDv7ForTest() {
	sessionid.ResetUUIDv7ForTest()
}
