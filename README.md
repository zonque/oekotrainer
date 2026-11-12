# kinobox

A live dashboard for the KinoBox pedal-power cinema. Two programs:

* **`kinobox`** – reads the data from a backend (a KinoBox on a serial port,
  or a simulator) and serves
  * `/` – a dashboard (riders, generated vs. consumed power, battery,
    protection status, 5-minute power chart), self-contained, works offline
  * `/api/readings` – the latest reading and energy totals as JSON
  * `/api/history` – generated/consumed power of the last 5 minutes
  * `/metrics` – Prometheus metrics
* **`kinobox-overlay`** – a borderless, always-on-top, click-through strip
  across the top (or bottom) of the screen that shows every rider's power and
  energy on top of a video player. It reads from the `kinobox` server.

The serial protocol is described in [docs/PROTOCOL.md](docs/PROTOCOL.md).

## Build and run

```sh
go build ./cmd/kinobox ./cmd/kinobox-overlay
cp kinobox.example.toml kinobox.toml   # set kinobox.device
./kinobox                              # reads ./kinobox.toml
./kinobox-overlay                      # same file, [overlay] section
```

Then open http://localhost:8080/ for the dashboard.

Both programs take the same flags:

| Flag      | Meaning                                                         |
|-----------|-----------------------------------------------------------------|
| `-config` | configuration file (default `kinobox.toml`; if that is missing, defaults are used) |
| `-debug`  | log at debug level                                              |

`kinobox-overlay` additionally has `-preview out.png` (and `-preview-width`),
which renders one frame from the server to a PNG instead of opening a window –
useful for trying out colours.

All other settings live in the TOML file; see
[kinobox.example.toml](kinobox.example.toml). Unknown keys are rejected.

## Backends

`backend` selects the data source; each backend has its own section.

* `kinobox` – a KinoBox on a serial port (`[kinobox]`, Linux only for now).
* `simulator` – synthetic data (`[simulator]`), for trying things out
  without hardware.

When `backend` is not set, `kinobox` is used if `kinobox.device` is set and
the simulator otherwise. New backends implement `backend.Backend` in
`internal/backend` and are wired up in `cmd/kinobox/main.go`.

The user running `kinobox` needs read/write access to the serial device
(on most distributions: membership in the `dialout` group).

## Overlay

The strip is configured in `[overlay]`: server URL, edge, height, monitor,
click-through, which parts to show, font, and a full colour theme in
`[overlay.colors]` (`#rrggbb` or `#rrggbbaa`). The overlay does not take the
keyboard focus from the player. With `click_through = false` it can be
closed with Escape; otherwise stop it with Ctrl-C or your task manager.

Building needs no C toolchain or development headers on any platform; the
overlay loads X11/OpenGL (Linux), Cocoa/Metal (macOS) or DirectX (Windows) at
runtime. Cross-compiling works too:

```sh
GOOS=windows go build -ldflags=-H=windowsgui ./cmd/kinobox-overlay
GOOS=darwin GOARCH=arm64 go build ./cmd/kinobox-overlay
```

Platform notes:

* **Linux/Wayland:** the overlay runs through XWayland. GNOME and KDE honour
  its position and always-on-top state; other compositors may not.
* **macOS:** the strip joins all Spaces, so it also shows above players in
  native fullscreen. If it does not, switch the player to non-native
  fullscreen (VLC: Preferences → Video → uncheck "Use the native fullscreen
  mode"). On notched MacBooks `position = "bottom"` avoids the notch. The
  binary is unsigned; allow it once via right-click → Open.

## Releases

Pushing a tag `vX.Y.Z` runs `.github/workflows/release.yml`, which tests the
code and has [GoReleaser](https://goreleaser.com) publish a GitHub release
with archives for Linux (amd64, arm64), Windows (amd64, arm64) and macOS
(universal binary), each containing both programs, the example config and the
docs, plus `checksums.txt`. Tags with a suffix (`v1.0.0-rc1`) become
pre-releases.

```sh
git tag v1.0.0 && git push origin v1.0.0
```

To try the release build locally without publishing:
`goreleaser release --snapshot --clean` (output in `dist/`).
