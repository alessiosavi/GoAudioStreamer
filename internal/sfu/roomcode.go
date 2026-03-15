package sfu

import "crypto/rand"

const roomCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// GenerateRoomCode returns a random room code in the format XXXX-XXXX.
// Uses crypto/rand with rejection sampling to avoid modular bias.
func GenerateRoomCode() string {
	code := make([]byte, 9) // XXXX-XXXX
	code[4] = '-'

	alphaLen := byte(len(roomCodeAlphabet)) // 31
	pos := 0
	for pos < 8 {
		var b [1]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic("crypto/rand failed: " + err.Error())
		}
		if b[0] >= 248 {
			continue
		}
		idx := pos
		if pos >= 4 {
			idx = pos + 1
		}
		code[idx] = roomCodeAlphabet[b[0]%alphaLen]
		pos++
	}
	return string(code)
}
