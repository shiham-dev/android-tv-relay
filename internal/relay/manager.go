// Package relay wires the ADB client to a TV registry and exposes it over an
// HTTP API and a simple TCP line protocol (the latter is what control systems
// such as Crestron talk to).
package relay

import (
	"crypto/rsa"
	"fmt"
	"sort"
	"sync"

	"github.com/shiham-dev/android-tv-relay/internal/adb"
	"github.com/shiham-dev/android-tv-relay/internal/config"
)

// Manager owns one ADB client per TV and serializes commands to each (adbd
// accepts a single client connection at a time).
type Manager struct {
	key     *rsa.PrivateKey
	keyName string
	tvs     map[string]*tvEntry
	order   []string
}

type tvEntry struct {
	name   string
	client *adb.Client
	mu     sync.Mutex // serialize commands to this TV
}

// NewManager builds a Manager from config and a loaded ADB key.
func NewManager(cfg config.Config, key *rsa.PrivateKey) *Manager {
	m := &Manager{
		key:     key,
		keyName: cfg.KeyName,
		tvs:     make(map[string]*tvEntry),
	}
	for _, tv := range cfg.TVs {
		m.tvs[tv.Name] = &tvEntry{
			name: tv.Name,
			client: &adb.Client{
				Addr:    tv.Addr,
				Key:     key,
				KeyName: cfg.KeyName,
			},
		}
		m.order = append(m.order, tv.Name)
	}
	sort.Strings(m.order)
	return m
}

// Names returns the configured TV names in stable order.
func (m *Manager) Names() []string {
	out := make([]string, len(m.order))
	copy(out, m.order)
	return out
}

// PublicKey returns the ADB public key (android format) used to authorize with
// TVs, so an operator can pre-approve it.
func (m *Manager) PublicKey() string {
	return string(adb.AndroidPublicKey(&m.key.PublicKey, m.keyName))
}

// resolve finds a TV by name. If name is empty and exactly one TV is
// configured, that TV is used.
func (m *Manager) resolve(name string) (*tvEntry, error) {
	if name == "" {
		if len(m.order) == 1 {
			return m.tvs[m.order[0]], nil
		}
		return nil, fmt.Errorf("no TV specified and %d are configured", len(m.order))
	}
	tv, ok := m.tvs[name]
	if !ok {
		return nil, fmt.Errorf("unknown TV %q", name)
	}
	return tv, nil
}

// Do runs a symbolic command (on/off/hdmi1/key:NN/...) against a TV.
func (m *Manager) Do(tvName, command string) error {
	tv, err := m.resolve(tvName)
	if err != nil {
		return err
	}
	if _, err := adb.Command(command); err != nil {
		return err
	}
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return tv.client.Do(command)
}

// Ping verifies a TV is reachable and authenticated.
func (m *Manager) Ping(tvName string) error {
	tv, err := m.resolve(tvName)
	if err != nil {
		return err
	}
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return tv.client.Ping()
}
