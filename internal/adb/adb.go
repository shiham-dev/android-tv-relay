// Package adb implements a minimal ADB-over-TCP client: just enough of the
// protocol to authenticate with an Android/Google TV and run a shell command
// (used here to inject key events). Pure standard library, no external deps.
package adb

import (
	"crypto"
	"crypto/rsa"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

var debug = os.Getenv("ADBDEBUG") != ""

func dbg(format string, a ...any) {
	if debug {
		fmt.Fprintf(os.Stderr, "[adb] "+format+"\n", a...)
	}
}

// ADB protocol command codes.
const (
	aCNXN = 0x4e584e43 // CNXN
	aAUTH = 0x48545541 // AUTH
	aOPEN = 0x4e45504f // OPEN
	aOKAY = 0x59414b4f // OKAY
	aWRTE = 0x45545257 // WRTE
	aCLSE = 0x45534c43 // CLSE
)

// AUTH subtypes.
const (
	authToken        = 1
	authSignature    = 2
	authRSAPublicKey = 3
)

const maxPayload = 1024 * 1024

// Client talks to a single TV's adbd.
type Client struct {
	Addr        string // host:port, e.g. 192.168.1.50:5555
	Key         *rsa.PrivateKey
	KeyName     string // label shown in the TV's "Allow debugging" dialog
	DialTimeout time.Duration
	IOTimeout   time.Duration
	PairTimeout time.Duration // time allowed for a human to tap "Allow" on first use
}

// Shell opens a shell service, runs command, drains its output and returns it.
// A fresh authenticated connection is used for each call: simple and robust for
// a relay that sends occasional one-shot commands.
func (c *Client) Shell(command string) (string, error) {
	conn, err := c.connect()
	if err != nil {
		return "", err
	}
	defer conn.raw.Close()

	const localID = 1
	if err := conn.send(aOPEN, localID, 0, []byte("shell:"+command+"\x00")); err != nil {
		return "", err
	}
	var out []byte
	for {
		m, err := conn.recv(c.ioTimeout())
		if err != nil {
			return string(out), err
		}
		if m.arg1 != localID {
			continue // not our stream
		}
		switch m.cmd {
		case aWRTE:
			out = append(out, m.data...)
			if err := conn.send(aOKAY, localID, m.arg0, nil); err != nil {
				return string(out), err
			}
		case aCLSE:
			_ = conn.send(aCLSE, localID, m.arg0, nil)
			return string(out), nil
		}
	}
}

// Ping verifies the TV is reachable and authenticated.
func (c *Client) Ping() error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	conn.raw.Close()
	return nil
}

type conn struct {
	raw net.Conn
	c   *Client
}

func (c *Client) connect() (*conn, error) {
	d := net.Dialer{Timeout: c.dialTimeout()}
	raw, err := d.Dial("tcp", c.Addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", c.Addr, err)
	}
	if tcp, ok := raw.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	cn := &conn{raw: raw, c: c}

	// Protocol version 0x01000001 pairs with a 1 MiB max payload; announcing the
	// older 0x01000000 with a payload above its 256 KiB cap makes adbd drop us.
	if err := cn.send(aCNXN, 0x01000001, maxPayload, []byte("host::features=cmd,shell_v2\x00")); err != nil {
		raw.Close()
		return nil, err
	}

	dbg("sent CNXN to %s", c.Addr)
	sentSig, sentPub := false, false
	wait := c.ioTimeout()
	for {
		m, err := cn.recv(wait)
		if err != nil {
			dbg("recv error: %v", err)
			raw.Close()
			return nil, err
		}
		dbg("recv cmd=%#x arg0=%d len=%d", m.cmd, m.arg0, len(m.data))
		switch {
		case m.cmd == aAUTH && m.arg0 == authToken:
			if !sentSig {
				sig, err := signToken(c.Key, m.data)
				if err != nil {
					raw.Close()
					return nil, err
				}
				if err := cn.send(aAUTH, authSignature, 0, sig); err != nil {
					raw.Close()
					return nil, err
				}
				sentSig = true
			} else if !sentPub {
				pub := AndroidPublicKey(&c.Key.PublicKey, c.keyName())
				if err := cn.send(aAUTH, authRSAPublicKey, 0, pub); err != nil {
					raw.Close()
					return nil, err
				}
				sentPub = true
				wait = c.pairTimeout() // waiting for the user to tap "Allow"
			} else {
				raw.Close()
				return nil, fmt.Errorf("TV rejected key %q (approve it on the TV's Allow dialog)", c.keyName())
			}
		case m.cmd == aCNXN:
			return cn, nil // connected and authenticated
		}
	}
}

// signToken signs the 20-byte auth token the way adb does: PKCS#1 v1.5 with the
// SHA-1 DigestInfo prefix wrapped around the token (which is itself treated as a
// SHA-1 digest). crypto/rsa does exactly this when given crypto.SHA1 and a
// 20-byte "hash".
func signToken(key *rsa.PrivateKey, token []byte) ([]byte, error) {
	return rsa.SignPKCS1v15(nil, key, crypto.SHA1, token)
}

type message struct {
	cmd        uint32
	arg0, arg1 uint32
	data       []byte
}

func (cn *conn) send(cmd, arg0, arg1 uint32, data []byte) error {
	var sum uint32
	for _, b := range data {
		sum += uint32(b)
	}
	var hdr [24]byte
	binary.LittleEndian.PutUint32(hdr[0:], cmd)
	binary.LittleEndian.PutUint32(hdr[4:], arg0)
	binary.LittleEndian.PutUint32(hdr[8:], arg1)
	binary.LittleEndian.PutUint32(hdr[12:], uint32(len(data)))
	binary.LittleEndian.PutUint32(hdr[16:], sum)
	binary.LittleEndian.PutUint32(hdr[20:], cmd^0xffffffff)

	_ = cn.raw.SetWriteDeadline(time.Now().Add(cn.c.ioTimeout()))
	if _, err := cn.raw.Write(hdr[:]); err != nil {
		return err
	}
	if len(data) > 0 {
		if _, err := cn.raw.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func (cn *conn) recv(timeout time.Duration) (message, error) {
	_ = cn.raw.SetReadDeadline(time.Now().Add(timeout))
	var hdr [24]byte
	if _, err := io.ReadFull(cn.raw, hdr[:]); err != nil {
		return message{}, err
	}
	m := message{
		cmd:  binary.LittleEndian.Uint32(hdr[0:]),
		arg0: binary.LittleEndian.Uint32(hdr[4:]),
		arg1: binary.LittleEndian.Uint32(hdr[8:]),
	}
	n := binary.LittleEndian.Uint32(hdr[12:])
	if n > maxPayload {
		return message{}, fmt.Errorf("oversized adb payload: %d", n)
	}
	if n > 0 {
		m.data = make([]byte, n)
		if _, err := io.ReadFull(cn.raw, m.data); err != nil {
			return message{}, err
		}
	}
	return m, nil
}

func (c *Client) dialTimeout() time.Duration {
	if c.DialTimeout > 0 {
		return c.DialTimeout
	}
	return 8 * time.Second
}

func (c *Client) ioTimeout() time.Duration {
	if c.IOTimeout > 0 {
		return c.IOTimeout
	}
	return 8 * time.Second
}

func (c *Client) pairTimeout() time.Duration {
	if c.PairTimeout > 0 {
		return c.PairTimeout
	}
	return 60 * time.Second
}

func (c *Client) keyName() string {
	if c.KeyName != "" {
		return c.KeyName
	}
	return "android-tv-relay"
}
