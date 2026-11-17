# Self-hosted music streaming server

## About
Build a music streaming server on your local computer and use it from outside the LAN. 

### Requirements
- Docker
- Tailscale account and an auth key

### Services
- refer `docker-compose.yml` for details

1. Navidrome (music streaming) -- `http://$TS_HOSTNAME:4533`
2. Jellyfin (video streaming) -- `http://$TS_HOSTNAME:8096`
3. Immich (photos/videos) -- `http://$TS_HOSTNAME:2283`, see [`immich/docker-compose.yml`](immich/docker-compose.yml)
4. Tailscale
    - Enable access from outside the LAN.
    - The apps share the `tailscale` container's network namespace
      (`network_mode: service:tailscale`), so they're all on one tailnet
      machine named `TS_HOSTNAME`, each at its own port.
    - Configured via `TS_AUTHKEY`/`TS_HOSTNAME` in `.env`.

## Usage
1. Get auth token from tailscale admin console
2. Edit `.env`
3. Run `docker compose up -d` at the root directory of this repo.

## Scripts
- In `/metadata-utils`, there is (are) some scripts to tidy metadata.
