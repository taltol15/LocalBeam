# LocalBeam

![Downloads](https://img.shields.io/github/downloads/taltol15/LocalBeam/total?style=for-the-badge&color=blue)

> **Fast. Secure. Local.**  
> Move files between computers on the same Wi‑Fi or LAN—no cloud, no accounts. **Windows, macOS, and Linux.**

![LocalBeam Screenshot](https://localbeam.net/app-screen.png)

## Overview

**LocalBeam** is a desktop app for peer-to-peer file transfer on your local network. Transfers use a direct HTTP connection; data stays on your LAN.

Built with **Go** and **React** (Vite) using **Wails v2**.

## Features (2.x)

- **Discovery:** UDP broadcast **plus** **mDNS** (Bonjour / DNS‑SD) so devices find each other more reliably across **Windows ↔ macOS** (and mixed home networks).
- **Manual address:** Send to an IP or `host:port` if discovery does not list a peer.
- **Security PIN:** A dynamic 4-digit PIN is required for every incoming transfer.
- **Progress:** Sender and receiver show upload/download progress.
- **Offline-first:** No internet required.
- **Streaming I/O:** Large files are streamed; memory use stays reasonable.
- **Protocol v2:** Versioned discovery payload and `X-LocalBeam-Version` header; `GET /localbeam/ping` for connectivity checks (useful for future mobile clients).

## Download

Prebuilt binaries are attached to **[GitHub Releases](https://github.com/taltol15/LocalBeam/releases)**.

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

## Build from source

1. Install [Go](https://go.dev/) and [Node.js](https://nodejs.org/) (LTS recommended).
2. Install Wails: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
3. Install platform dependencies for Wails ([official guide](https://wails.io/docs/gettingstarted/installation)). On Debian/Ubuntu you typically need `libgtk-3-dev` and **`libwebkit2gtk-4.0-dev`** (Wails expects the `webkit2gtk-4.0` pkg-config name).
4. Clone and run:

   ```bash
   git clone https://github.com/taltol15/LocalBeam.git
   cd LocalBeam
   go mod tidy
   cd frontend && npm install && cd ..
   wails dev
   ```

5. Production build:

   ```bash
   wails build
   ```

   Output is under `build/bin/`. On macOS, for a universal binary:  
   `wails build -platform darwin/universal`

## Automated release builds

Pushing a **git tag** matching `v*` (for example `v2.0.0`) runs [`.github/workflows/release.yml`](.github/workflows/release.yml): it builds on **Windows, macOS, and Linux**, uploads artifacts, and creates a **GitHub Release** with those files.

**Create a release from your machine:**

```bash
git tag v2.0.0
git push origin v2.0.0
```

You can also run the workflow manually from the **Actions** tab (**workflow_dispatch**) to verify builds without creating a tag (the *publish release* step only runs for `refs/tags/v*`).

## License

MIT License.

---

*Developed with care by Tal*
