# LocalBeam

![Downloads](https://img.shields.io/github/downloads/Trustity/LocalBeam/total?style=for-the-badge&color=3dff8a)

> **A Trustity Labs experiment**  
> Encrypted peer-to-peer file transfer on your Wi‑Fi / LAN — no cloud, no accounts. **Windows, macOS, and Linux.**

LocalBeam is a Trustity Labs project. It is **not** a production Trustity product.

## Overview

**LocalBeam** moves files between computers on the same local network over a direct connection. Protocol **v3** adds transport TLS, AES-GCM payload encryption, a PIN challenge (PIN is never sent in cleartext), required claimed sender identity for local audit, and **manual Accept / Reject** on the receiver.

Built with **Go** and **React** (Vite) using **Wails v2**.

## Security (v3)

| Control | Behavior |
|--------|----------|
| **HTTPS** | Ephemeral self-signed TLS on the receiver; fingerprint shown in UI / discovery |
| **AES-256-GCM** | File stream encrypted; key from Argon2id(PIN, salt) + HKDF(transfer ID) |
| **PIN** | Crypto-random **6-digit** receive PIN; challenge–response HMAC (not sent as a header) |
| **Consent** | Receiver must Accept before upload; tip: *Verify with the sender that they are the intended sender of this file.* |
| **Identity (claimed)** | Sender must provide name + organizational email — logged locally for audit, **not** cryptographically proven |
| **Audit log** | JSONL on the receiver machine only (no central Trustity logging in Labs) |

PIN rotates after reject or successful receive.

## Features

- **Discovery:** UDP broadcast + **mDNS** (Bonjour / DNS‑SD)
- **Manual address:** Send to an IP or `host:port` if discovery misses a peer
- **Progress:** Upload / download progress on both sides
- **Offline-first:** No internet required for transfer
- **Streaming I/O:** Chunked encrypt/decrypt for large files

## Download

Prebuilt binaries are attached to **[GitHub Releases](https://github.com/Trustity/LocalBeam/releases)**.

| Asset | Platform |
|--------|-----------|
| `localbeam-windows-amd64.zip` | Windows (amd64) |
| `localbeam-macos-universal.zip` | macOS (Intel + Apple Silicon) |
| `localbeam-linux-amd64.tar.gz` | Linux (amd64) |

**macOS:** Open the app from the zip; if Gatekeeper blocks it, use **System Settings → Privacy & Security** or right‑click → Open the first time.

**Firewall:** Allow LocalBeam on private networks if prompted (incoming connections on the transfer port are required to receive files).

## Tech stack

- **Backend:** Go
- **Frontend:** React + Vite
- **Desktop shell:** Wails v2
- **Discovery:** UDP + [zeroconf](https://github.com/grandcat/zeroconf) (mDNS)
- **Crypto:** TLS 1.2+, Argon2id, HKDF-SHA256, AES-256-GCM

## Build from source

1. Install [Go](https://go.dev/) and [Node.js](https://nodejs.org/) (LTS recommended).
2. Install Wails: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
3. Install platform dependencies for Wails ([official guide](https://wails.io/docs/gettingstarted/installation)). On Debian/Ubuntu you need `libgtk-3-dev`. For WebKitGTK, use **`libwebkit2gtk-4.1-dev`** and build with **`wails build -tags webkit2_41`** on distros that only ship WebKit 4.1. On older systems with **`libwebkit2gtk-4.0-dev`**, a plain `wails build` is enough.
4. Clone and run:

   ```bash
   git clone https://github.com/Trustity/LocalBeam.git
   cd LocalBeam
   go mod tidy
   cd frontend && npm install && npm run build && cd ..
   wails dev
   ```

5. Production build:

   ```bash
   wails build
   ```

   Output is under `build/bin/`. On macOS, for a universal binary:  
   `wails build -platform darwin/universal`

## Automated release builds

Pushing a **git tag** matching `v*` (for example `v3.0.0`) runs [`.github/workflows/release.yml`](.github/workflows/release.yml): it builds on **Windows, macOS, and Linux**, uploads artifacts, and creates a **GitHub Release**.

```bash
git tag v3.0.0
git push origin v3.0.0
```

## License

MIT License.

---

*Trustity Labs · LocalBeam*
