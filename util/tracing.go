package util

func TracePayload(payload []byte) []byte {
	max := 10
	length := len(payload)
	if length < max {
		max = length
	}
	return payload[:max]
}
