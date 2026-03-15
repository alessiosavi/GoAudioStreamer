package sfu

import (
	"strings"
	"testing"
)

func TestGenerateRoomCode_Format(t *testing.T) {
	code := GenerateRoomCode()
	if len(code) != 9 {
		t.Fatalf("code length: got %d, want 9: %q", len(code), code)
	}
	if code[4] != '-' {
		t.Fatalf("code[4]: got %q, want '-': %q", code[4], code)
	}
}

func TestGenerateRoomCode_Alphabet(t *testing.T) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	for i := 0; i < 100; i++ {
		code := GenerateRoomCode()
		chars := strings.ReplaceAll(code, "-", "")
		for _, c := range chars {
			if !strings.ContainsRune(alphabet, c) {
				t.Fatalf("code contains invalid char %q: %q", c, code)
			}
		}
	}
}

func TestGenerateRoomCode_NoAmbiguous(t *testing.T) {
	for i := 0; i < 100; i++ {
		code := GenerateRoomCode()
		for _, bad := range "0OoIiLl1" {
			if strings.ContainsRune(code, bad) {
				t.Fatalf("code contains ambiguous char %q: %q", bad, code)
			}
		}
	}
}

func TestGenerateRoomCode_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code := GenerateRoomCode()
		if seen[code] {
			t.Fatalf("duplicate code after %d generations: %q", i, code)
		}
		seen[code] = true
	}
}
