// Command android-tv-relay is a small network service that controls Android /
// Google TVs over ADB (power, HDMI input, key events) and exposes them through
// an HTTP API and a plain-text TCP line protocol for AV control systems.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shiham-dev/android-tv-relay/internal/adb"
	"github.com/shiham-dev/android-tv-relay/internal/config"
	"github.com/shiham-dev/android-tv-relay/internal/relay"
)

var version = "dev" // set via -ldflags at build time

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	var (
		configPath = flag.String("config", os.Getenv("CONFIG"), "path to JSON config file (optional)")
		showVer    = flag.Bool("version", false, "print version and exit")
		pubkey     = flag.Bool("pubkey", false, "print the ADB public key and exit")
		send       = flag.String("send", "", "send one command and exit (format: [tv] command)")
	)
	flag.Parse()

	if *showVer {
		fmt.Println(version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	key, err := adb.LoadOrCreateKey(cfg.KeyPath)
	if err != nil {
		log.Fatalf("adb key: %v", err)
	}
	mgr := relay.NewManager(cfg, key)

	if *pubkey {
		fmt.Print(mgr.PublicKey())
		return
	}
	if *send != "" {
		tv, command := splitSend(*send)
		if err := mgr.Do(tv, command); err != nil {
			log.Fatalf("send: %v", err)
		}
		fmt.Println("OK")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("android-tv-relay %s starting; TVs=%v", version, mgr.Names())

	errCh := make(chan error, 2)
	var httpSrv *http.Server

	if cfg.HTTPAddr != "" {
		httpSrv = &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           mgr.HTTPHandler(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			log.Printf("HTTP API listening on %s", cfg.HTTPAddr)
			if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("http: %w", err)
			}
		}()
	}

	if cfg.TCPAddr != "" {
		ln, err := net.Listen("tcp", cfg.TCPAddr)
		if err != nil {
			log.Fatalf("tcp listen: %v", err)
		}
		go func() {
			log.Printf("TCP line protocol listening on %s", cfg.TCPAddr)
			if err := mgr.ServeTCP(ctx, ln); err != nil {
				errCh <- fmt.Errorf("tcp: %w", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		log.Print("shutting down")
	case err := <-errCh:
		log.Printf("server error: %v", err)
	}

	if httpSrv != nil {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}
}

// splitSend parses the -send argument. "command" targets the sole TV;
// "tv command" targets a named TV.
func splitSend(s string) (tv, command string) {
	fields := strings.Fields(s)
	if len(fields) >= 2 {
		return fields[0], strings.Join(fields[1:], " ")
	}
	return "", strings.TrimSpace(s)
}
