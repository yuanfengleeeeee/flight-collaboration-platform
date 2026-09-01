// Package id contains public identifier helpers shared by Core and Edge.
package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewPublicID returns a UUIDv7-compatible identifier. The first 48 bits are
// the UTC Unix millisecond timestamp, followed by random bits and the UUIDv7
// version/variant markers.
func NewPublicID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate public id entropy: %w", err)
	}

	millis := uint64(time.Now().UTC().UnixMilli())
	value[0] = byte(millis >> 40)
	value[1] = byte(millis >> 32)
	value[2] = byte(millis >> 24)
	value[3] = byte(millis >> 16)
	value[4] = byte(millis >> 8)
	value[5] = byte(millis)
	value[6] = (value[6] & 0x0f) | 0x70
	value[8] = (value[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(value[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]), nil
}

// MustPublicID is intended for tests and development fixtures.
func MustPublicID() string {
	value, err := NewPublicID()
	if err != nil {
		panic(err)
	}
	return value
}
