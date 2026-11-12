package relay

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// ServeTCP runs the line protocol until ctx is cancelled.
//
// One command per line (CRLF or LF). Grammar:
//
//	<command>              when a single TV is configured
//	<tv> <command>         to target a named TV
//	PING                   health check  -> "OK PING"
//	LIST                   list TV names  -> "OK <name> <name> ..."
//
// Commands: on off power hdmi1..hdmi4 home back volup voldown mute key:<code>.
// Replies: "OK ..." or "ERR <reason>". This is the port control systems such as
// Crestron connect to with a TCP/IP Client symbol.
func (m *Manager) ServeTCP(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go m.handleTCP(conn)
	}
}

func (m *Manager) handleTCP(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 1024), 64*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		reply := m.handleLine(line)
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprintf(conn, "%s\r\n", reply); err != nil {
			return
		}
	}
}

func (m *Manager) handleLine(line string) string {
	upper := strings.ToUpper(line)
	switch upper {
	case "PING":
		return "OK PING"
	case "LIST":
		return "OK " + strings.Join(m.Names(), " ")
	}

	tv, command := "", line
	if fields := strings.Fields(line); len(fields) >= 2 {
		// First token is a TV name only if it matches a configured TV;
		// otherwise treat the whole line as a command for the single TV.
		if _, ok := m.tvs[fields[0]]; ok {
			tv = fields[0]
			command = strings.Join(fields[1:], " ")
		}
	}
	if err := m.Do(tv, command); err != nil {
		log.Printf("tcp command failed: tv=%q cmd=%q: %v", tv, command, err)
		return "ERR " + err.Error()
	}
	return "OK " + strings.ToUpper(command)
}
