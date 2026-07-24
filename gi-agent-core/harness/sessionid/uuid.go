package sessionid

import llm "github.com/nowa/gi/gi-llm-provider"

type RandomBytesFunc = llm.RandomBytesFunc
type NowMillisFunc = llm.NowMillisFunc

func UUIDv7() string {
	return llm.UUIDv7()
}

func UUIDv7With(randomBytes RandomBytesFunc, nowMillis NowMillisFunc) string {
	return llm.UUIDv7With(randomBytes, nowMillis)
}

func ResetUUIDv7ForTest() {
	llm.ResetUUIDv7ForTest()
}
