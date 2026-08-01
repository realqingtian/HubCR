package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

var ErrInvalidID = errors.New("invalid ID")

// ID is an opaque UUID used by auth domain records.
type ID string

func NewID() (ID, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate ID: %w", err)
	}

	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80

	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return ID(encoded), nil
}

func ParseID(value string) (ID, error) {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return "", ErrInvalidID
	}

	compact := make([]byte, 32)
	copy(compact[0:8], value[0:8])
	copy(compact[8:12], value[9:13])
	copy(compact[12:16], value[14:18])
	copy(compact[16:20], value[19:23])
	copy(compact[20:32], value[24:36])
	decoded := make([]byte, 16)
	if _, err := hex.Decode(decoded, compact); err != nil {
		return "", ErrInvalidID
	}
	if decoded[6]>>4 != 4 || decoded[8]>>6 != 2 {
		return "", ErrInvalidID
	}
	return ID(value), nil
}
