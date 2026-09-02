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

Export the printed argon2id hash (and OIDC secret if used), then start:

```bash
export KIOSK_PASSWORD_HASH='...'          # optional if using OIDC only
export KIOSK_OIDC_CLIENT_SECRET='...'     # optional if using password only
kiosk-display -config ~/.config/kiosk-display/config.yaml
```

Default config path if `-config` is omitted: `~/.config/kiosk-display/config.yaml`.

Runtime data (TLS certs, Chrome profile, links store, audit log) lives under `~/.local/share/kiosk-display`.

At least one of password auth (`KIOSK_PASSWORD_HASH` / `manager.password_hash`) or `manager.oidc` is required. Both can be enabled together. Env vars override YAML when set.

## Config

See `config.example.yaml`.


| Section                                           | Purpose                                                        |
| ------------------------------------------------- | -------------------------------------------------------------- |
| `manager`                                         | Bind/host/port for the admin UI; auth via password and/or OIDC |
| `display`                                         | Loopback display UI port; `sleep` / `wake` shell command lists |
| `chrome`                                          | Browser binary, `DISPLAY`, resolution, restart delay           |
| `links`                                           | Seed bookmarks shown on the kiosk                              |
| `allowed_hosts`                                   | Navigation allowlist                                           |
| `allow_edit_links`                                | Whether the manager can change links at runtime                |
| `auto_allow_link_hosts` / `auto_allow_subdomains` | Expand allowlist from configured links                         |




### Manager auth


| Key / env                               | Purpose                                                      |
| --------------------------------------- | ------------------------------------------------------------ |
| `password_hash` / `KIOSK_PASSWORD_HASH` | Argon2id hash for password login (optional if `oidc` is set) |
| `oidc`                                  | OpenID Connect (OAuth 2.0) login via an IdP such as Authelia |




#### `manager.oidc`


| Key / env                                    | Purpose                                                                                        |
| -------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| `issuer`                                     | IdP issuer URL (discovery at `{issuer}/.well-known/openid-configuration`)                      |
| `client_id`                                  | OIDC client ID                                                                                 |
| `client_secret` / `KIOSK_OIDC_CLIENT_SECRET` | OIDC client secret                                                                             |
| `redirect_url`                               | Callback URL (`…/api/oidc/callback`); must match the IdP client registration                   |
| `post_logout_redirect_url`                   | Where to land after IdP logout (optional; defaults to origin of `redirect_url`)                |
| `scopes`                                     | Scopes to request (default `openid profile email`; `groups` added if `required_groups` is set) |
| `required_groups`                            | If set, ID token must include at least one of these groups                                     |
| `auto_redirect`                              | When true, unauthenticated visits go straight to the IdP                                       |


Logout uses `end_session_endpoint` from discovery when present; otherwise falls back to `{issuer}/logout?rd=…` (Authelia portal logout).

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



### Pre-commit checks

Optional git hook runs the same lint/vuln checks as CI:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
go install golang.org/x/vuln/cmd/govulncheck@v1.7.0
git config core.hooksPath .githooks
```

Or run manually: `make check` (`lint` + `vuln`).