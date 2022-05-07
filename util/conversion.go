package util

import (
	"bytes"
	"encoding/gob"
)

// container needs to be a pointer
func Decode(buf []byte, container interface{}) error {
	err := gob.NewDecoder(bytes.NewBuffer(buf)).Decode(container)
	if err != nil {
		return err
	}
	return nil
}

func Encode(msg interface{}) []byte {
	var buf bytes.Buffer
	gob.NewEncoder(&buf).Encode(msg)
	return buf.Bytes()
}
