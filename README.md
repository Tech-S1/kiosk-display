# Kiosk Display

Fullscreen Chromium kiosk for a rack or wall display

## Install

Download a binary from [Releases](https://github.com/Tech-S1/kiosk-display/releases) (`linux`/`darwin`, `amd64`/`arm64`) and verify with `SHA256SUMS`.

```bash
chmod +x kiosk-display_*
sudo mv kiosk-display_* /usr/local/bin/kiosk-display
```

## Requirements

- Chromium on the kiosk host
- X11 display for the browser (`chrome.display`, usually `:0`)

## Quick start

```bash
cp config.example.yaml ~/.config/kiosk-display/config.yaml
kiosk-display -hash-password 'your-password'
```

Put the printed argon2id hash into `manager.password_hash`, then:

```bash
kiosk-display -config ~/.config/kiosk-display/config.yaml
```

Default config path if `-config` is omitted: `~/.config/kiosk-display/config.yaml`.

Runtime data (TLS certs, Chrome profile, links store, audit log) lives under `~/.local/share/kiosk-display`.

## Config

See `config.example.yaml`.


| Section                                           | Purpose                                                        |
| ------------------------------------------------- | -------------------------------------------------------------- |
| `manager`                                         | Bind/host/port for the admin UI; `password_hash` required      |
| `display`                                         | Loopback display UI port; `sleep` / `wake` shell command lists |
| `chrome`                                          | Browser binary, `DISPLAY`, resolution, restart delay           |
| `links`                                           | Seed bookmarks shown on the kiosk                              |
| `allowed_hosts`                                   | Navigation allowlist                                           |
| `allow_edit_links`                                | Whether the manager can change links at runtime                |
| `auto_allow_link_hosts` / `auto_allow_subdomains` | Expand allowlist from configured links                         |


## Flags


| Flag                    | Description                                                 |
| ----------------------- | ----------------------------------------------------------- |
| `-config path`          | Config file (default `~/.config/kiosk-display/config.yaml`) |
| `-hash-password string` | Print argon2id hash and exit                                |


## Develop

Needs Go 1.26+.

```bash
make build
./bin/kiosk-display -config ./config.yaml
```

