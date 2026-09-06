# Self-hosted music streaming server

## About
Build a music streaming server on your local computer and use it from outside the LAN. 

### Requirements
- Docker
- Tailscale account and an auth key

### Services
- refer `docker-compose.yml` for details

1. Navidrome
    - Music streaming
    - config: [`/filebrowser-data/config.yaml`](/filebrowser-data/config.yaml)

2. FileBrowser Quantum (https://github.com/gtsteffaniak/filebrowser)
    - Uploading music files
    - config: [`/navidrome-data/navidrome.toml`](/navidrome-data/navidrome.toml)

3. Tailscale
    - Enable access from outside the LAN.
    - Not a separate service: it's embedded into the `landing` container via
      [tsnet](https://pkg.go.dev/tailscale.com/tsnet), which is also the
      reverse proxy for Navidrome/File Browser/Jellyfin and the dashboard
      shown at `/`. See [`landing/README.md`](landing/README.md).
    - Configured via `TS_AUTHKEY`/`TS_HOSTNAME` in `.env`. If `TS_AUTHKEY`
      is left empty, `landing` just runs as a plain local HTTP server
      instead of joining a tailnet -- remove/unset it if you don't need
      access from outside the LAN.

## Usage
1. Get auth token from tailscale admin console
2. Edit `.env`
3. Run `docker compose up -d` at the root directory of this repo.

## Enabling HTTPS (future work)

`landing` doesn't terminate TLS yet -- it's served plain over the tailnet.
Tailscale can issue and auto-renew certificates for a node's own
`*.ts.net` name, and `tsnet` can consume them directly, so enabling this
later needs no reverse proxy or manual certificate handling:

1. In the Tailscale admin console, enable **HTTPS Certificates** for this
   tailnet (DNS settings) and make sure **MagicDNS** is on, so `landing`
   gets a valid `<hostname>.<tailnet>.ts.net` name.
2. In [`landing/cmd/landing/main.go`](landing/cmd/landing/main.go), change the dashboard listener
   from `srv.Listen("tcp", ":80")` to `srv.ListenTLS("tcp", ":443")` (or
   `srv.ListenFunnel(...)` if it should also be reachable from the public
   internet, not just the tailnet).
3. Nothing else to configure: `tsnet`'s local client fetches and renews the
   certificate for `srv.CertDomains()` automatically.
4. Optionally redirect `:80` to `:443`, or drop the plain `:80` listener
   entirely once HTTPS is confirmed working.

## Scripts
- In `/metadata-utils`, there is (are) some scripts to tidy metadata.
