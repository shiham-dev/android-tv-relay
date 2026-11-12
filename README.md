# android-tv-relay

Control **Android TV / Google TV** sets (TCL, Philips, Sony, Hisense, and other
Android-based TVs) over the network — power, HDMI input, and any key event —
from AV control systems, scripts, or a browser.

It speaks the TV's own **ADB** protocol (including the RSA authentication
handshake) in pure Go, and exposes two front ends:

- a small **HTTP REST API**, and
- a plain-text **TCP line protocol** designed for AV control processors such as
  **Crestron**, **AMX**, or **Control4**, which can open a TCP socket and send a
  word.

No `adb` binary, no Android SDK, no cloud, no third-party Go dependencies — a
single ~6 MB static binary or a distroless container.

> **Why this exists:** consumer Android/Google TVs expose no simple RS-232, CEC,
> or HTTP control — the only reliable network control path is ADB, which
> requires an RSA-signed handshake that control systems can't perform on their
> own. This relay does the handshake and gives them a trivial protocol instead.

---

## Features

- Power on/off (wake/sleep), discrete HDMI 1–4, and arbitrary Android key codes.
- Multiple TVs from one instance, addressed by name.
- Pure standard library: `crypto/rsa` + `math/big` implement the ADB auth; no CGO.
- Persistent ADB key so the TV's "Allow debugging" prompt appears only once.
- HTTP API **and** Crestron-friendly TCP protocol, either or both.
- Ships as a static binary, a systemd unit, a Docker image, and Kubernetes manifests.

---

## Quick start

### Docker

```bash
docker run --rm -p 8080:8080 -p 8700:8700 \
  -e TVS="living=192.168.1.50" \
  -v tvrelay-data:/data \
  ghcr.io/shiham-dev/android-tv-relay:latest
```

### Binary

```bash
go build -o android-tv-relay ./cmd/android-tv-relay
TVS="living=192.168.1.50" ./android-tv-relay
```

Then:

```bash
curl -X POST http://localhost:8080/api/tv/living/power/on
curl -X POST http://localhost:8080/api/tv/living/input/2      # HDMI 2
printf 'living hdmi1\r\n' | nc localhost 8700                 # TCP protocol
```

### First run: authorize the key

On the **first** command to a TV, the set shows an **"Allow USB/network
debugging?"** dialog. Tick *Always allow from this computer* and accept. The
relay stores its key (default `/data/adbkey`) and reuses it, so this happens
only once per TV. The TV must have **Developer options → USB debugging /
network debugging** enabled.

Print the public key to pre-authorize or inspect it:

```bash
./android-tv-relay -pubkey
```

---

## Configuration

Config comes from an optional JSON file (`-config` / `CONFIG`), with environment
overrides. See [`config.example.json`](config.example.json).

| Env | Default | Meaning |
|-----|---------|---------|
| `TVS` | – | `name=addr,name2=addr2` (port defaults to `:5555`) |
| `TV_ADDR` / `TV_NAME` | – | single-TV shortcut |
| `HTTP_ADDR` | `:8080` | REST API listen address (`""` disables) |
| `TCP_ADDR` | `:8700` | line-protocol listen address (`""` disables) |
| `ADB_KEY_PATH` | `/data/adbkey` | where the ADB private key is stored/generated |
| `ADB_KEY_NAME` | `android-tv-relay` | label shown in the TV's Allow dialog |
| `CONFIG` | – | path to a JSON config file |

---

## HTTP API

| Method & path | Action |
|---------------|--------|
| `GET  /healthz`, `/readyz` | health checks |
| `GET  /api/tvs` | list configured TV names |
| `GET  /api/pubkey` | ADB public key (to pre-authorize) |
| `POST /api/tv/{name}/power/{on\|off\|toggle}` | power |
| `POST /api/tv/{name}/input/{1-4}` | HDMI input |
| `POST /api/tv/{name}/key/{code}` | raw Android key code |
| `POST /api/tv/{name}/cmd/{command}` | any symbolic command |
| `POST /api/cmd/{command}` | shortcut when one TV is configured |

Responses are JSON: `{"ok":true,"tv":"living","command":"hdmi1"}` or HTTP 502
with `{"ok":false,...,"error":"..."}`.

## TCP line protocol (for Crestron / AV systems)

Connect to `TCP_ADDR` (default `:8700`) and send one command per line
(`\r\n` or `\n`). Reply is `OK ...` or `ERR <reason>`.

```
PING                 -> OK PING
LIST                 -> OK living lobby
hdmi1                -> OK HDMI1          (when one TV is configured)
living hdmi2         -> OK hdmi2          (named TV)
living off           -> OK off
living key:66        -> OK key:66
```

**Commands:** `on` `off` `power`/`toggle` `hdmi1`..`hdmi4` `home` `back`
`volup` `voldown` `mute` `key:<code>`.

### Crestron wiring

Use a **TCP/IP Client** symbol pointed at the relay's address and port `8700`,
and send the command strings above (append `\x0D\x0A`) from your button presses.
Watch the returned `OK`/`ERR` line for feedback.

---

## Deploy

### Ubuntu (systemd)

See [`deploy/systemd/android-tv-relay.service`](deploy/systemd/android-tv-relay.service)
for an install recipe and a hardened unit.

### Kubernetes

```bash
kubectl apply -f deploy/k8s/configmap.yaml
kubectl create secret generic android-tv-relay-key --from-file=adbkey=./adbkey
kubectl apply -f deploy/k8s/deployment.yaml -f deploy/k8s/service.yaml
```

Manifests in [`deploy/k8s`](deploy/k8s): a single-replica Deployment (adbd
allows one client per TV), the ADB key as a Secret, and a LoadBalancer Service
exposing HTTP (8080) and TCP (8700). The pod must be able to route to the TVs'
LAN on TCP 5555 — use `hostNetwork: true` or appropriate routes if your CNI
isolates the pod network.

---

## Notes & limitations

- **Wake over Wi‑Fi:** a TV that sleeps on Wi‑Fi may drop off the network and
  miss a wake command. Enable the TV's *Wake on Wi‑Fi / network standby*, or use
  a wired connection, for reliable power-on.
- **One client per TV:** adbd serves a single ADB connection at a time. The relay
  serializes commands per TV; don't also run `adb` against the same set.
- **Security:** anyone who can reach the API/TCP port can control the TVs. Keep
  it on a trusted VLAN and/or front it with network policy. The ADB private key
  grants debug access to the TVs — treat it as a secret (it is `.gitignore`d).

## Build

```bash
make build        # ./bin/android-tv-relay
make test vet     # tests + vet
make docker       # container image
```

## License

MIT — see [LICENSE](LICENSE).
