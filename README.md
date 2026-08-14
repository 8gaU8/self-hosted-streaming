# Self-hosted music streaming server

## Services
- refer `docker-compose.yml`

1. Navidrome
    - Music streaming
    - config: [`/filebrowser-data/config.yaml`](/filebrowser-data/config.yaml)

2. FileBrowser Quantum (https://github.com/gtsteffaniak/filebrowser)
    - Uploading music files
    - config: [`/navidrome-data/navidrome.toml`](/navidrome-data/navidrome.toml)

3. Tailscale
    - Enable access from outside the LAN.
    - Remove this service and related configurations from above 2 services if not necessary
    - config: [`/ts-config/serve.json`](/ts-config/serve.json)

## Usage
1. Get auth token from tailscale admin console
2. Edit `.env`
3. Run `docker compose up -d` at the root directory of this repo.

## Scripts
- In `/metadata-utils`, there is (are) some scripts to tidy metadata.