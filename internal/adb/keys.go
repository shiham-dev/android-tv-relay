package adb

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
)

// KeySize is the RSA modulus size ADB uses.
const (
	keyBits      = 2048
	modulusBytes = keyBits / 8 // 256
	modulusWords = modulusBytes / 4
)

// LoadOrCreateKey loads an ADB RSA private key from path, generating and
// persisting a new one if the file does not exist. The companion public key
// (ADB "android" format) is written next to it as <path>.pub.
func LoadOrCreateKey(path string) (*rsa.PrivateKey, error) {
	if path == "" {
		return rsa.GenerateKey(rand.Reader, keyBits)
	}
	if data, err := os.ReadFile(path); err == nil {
		key, err := parsePrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("parse adb key %s: %w", path, err)
		}
		return key, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8(key),
	})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path+".pub", AndroidPublicKey(&key.PublicKey, "android-tv-relay"), 0o644); err != nil {
		return nil, err
	}
	return key, nil
}

func parsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA key")
	}
	return rsaKey, nil
}

func pkcs8(key *rsa.PrivateKey) []byte {
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		// MarshalPKCS8PrivateKey never fails for a valid RSA key.
		panic(err)
	}
	return b
}

// AndroidPublicKey encodes an RSA public key in the binary format adbd expects
// in an A_AUTH RSAPUBLICKEY packet: base64(struct) + " " + name + "\x00".
//
// The struct (little-endian) is:
//
//	uint32 modulus_size_words (64)
//	uint32 n0inv              (-1 / n[0] mod 2^32)
//	uint8  modulus[256]       (little-endian)
//	uint8  rr[256]            (R^2 mod n, little-endian, R = 2^2048)
//	uint32 exponent
func AndroidPublicKey(pub *rsa.PublicKey, name string) []byte {
	buf := make([]byte, 0, 4+4+modulusBytes+modulusBytes+4)

	n := pub.N
	r32 := new(big.Int).Lsh(big.NewInt(1), 32)

	// n0inv = -(n^-1) mod 2^32
	n0 := new(big.Int).Mod(n, r32)
	n0inv := new(big.Int).ModInverse(n0, r32)
	n0inv.Sub(r32, n0inv)
	n0inv.Mod(n0inv, r32)

	// rr = R^2 mod n, R = 2^(32*words)
	rr := new(big.Int).Lsh(big.NewInt(1), uint(modulusWords*32*2))
	rr.Mod(rr, n)

	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], uint32(modulusWords))
	buf = append(buf, u32[:]...)
	binary.LittleEndian.PutUint32(u32[:], uint32(n0inv.Uint64()))
	buf = append(buf, u32[:]...)
	buf = append(buf, littleEndianBytes(n, modulusBytes)...)
	buf = append(buf, littleEndianBytes(rr, modulusBytes)...)
	binary.LittleEndian.PutUint32(u32[:], uint32(pub.E))
	buf = append(buf, u32[:]...)

	encoded := base64.StdEncoding.EncodeToString(buf)
	return []byte(encoded + " " + name + "\x00")
}

func littleEndianBytes(v *big.Int, size int) []byte {
	be := v.Bytes() // big-endian, no leading zeros
	out := make([]byte, size)
	for i := 0; i < len(be) && i < size; i++ {
		out[i] = be[len(be)-1-i]
	}
	return out
}
