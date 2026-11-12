package adb

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"math/big"
	"strings"
	"testing"
)

func TestCommand(t *testing.T) {
	cases := map[string]int{
		"on": KeyWakeup, "poweron": KeyWakeup,
		"off": KeySleep, "poweroff": KeySleep,
		"power": KeyPower, "toggle": KeyPower,
		"hdmi1": KeyHDMI1, "hdmi2": KeyHDMI2, "hdmi3": KeyHDMI3, "hdmi4": KeyHDMI4,
		"HDMI1": KeyHDMI1, " hdmi2 ": KeyHDMI2,
		"home": KeyHome, "back": KeyBack, "mute": KeyMute,
		"key:66": 66, "42": 42,
	}
	for in, want := range cases {
		got, err := Command(in)
		if err != nil {
			t.Errorf("Command(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Command(%q) = %d, want %d", in, got, want)
		}
	}
	for _, bad := range []string{"", "nope", "key:", "-1", "9999"} {
		if _, err := Command(bad); err == nil {
			t.Errorf("Command(%q) expected error", bad)
		}
	}
}

// TestAndroidPublicKey verifies the encoded modulus round-trips back to the key.
func TestAndroidPublicKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		t.Fatal(err)
	}
	line := AndroidPublicKey(&key.PublicKey, "test@host")
	s := string(line)
	if !strings.HasSuffix(s, " test@host\x00") {
		t.Fatalf("missing name suffix: %q", s)
	}
	b64 := strings.SplitN(s, " ", 2)[0]
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	// struct: [0:4] words, [4:8] n0inv, [8:8+256] modulus LE, ... , last 4 exponent
	if len(raw) != 4+4+modulusBytes+modulusBytes+4 {
		t.Fatalf("unexpected struct size %d", len(raw))
	}
	if words := binary.LittleEndian.Uint32(raw[0:]); words != modulusWords {
		t.Fatalf("modulus words = %d, want %d", words, modulusWords)
	}
	modLE := raw[8 : 8+modulusBytes]
	be := make([]byte, modulusBytes)
	for i := 0; i < modulusBytes; i++ {
		be[i] = modLE[modulusBytes-1-i]
	}
	gotN := new(big.Int).SetBytes(be)
	if gotN.Cmp(key.PublicKey.N) != 0 {
		t.Fatal("decoded modulus does not match key")
	}
	if exp := binary.LittleEndian.Uint32(raw[len(raw)-4:]); int(exp) != key.PublicKey.E {
		t.Fatalf("exponent = %d, want %d", exp, key.PublicKey.E)
	}
}

func TestLoadOrCreateKeyEphemeral(t *testing.T) {
	key, err := LoadOrCreateKey("")
	if err != nil {
		t.Fatal(err)
	}
	if key.N.BitLen() < keyBits-8 {
		t.Fatalf("unexpected key size %d", key.N.BitLen())
	}
}
