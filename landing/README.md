# landing2

Flask-based landing/dashboard page for the media server. This is the sole
entry point on port 80 (the old nginx `landing` service has been retired).

- `/` — dashboard: icon links to Jellyfin/Navidrome/File Browser, live
  status dots, and per-container + stack-wide CPU/memory/disk I/O usage.
- `/api/status` — JSON health check for the linked services.
- `/api/usage` — JSON CPU/memory/disk I/O usage per service plus a stack-wide
  total, read from the Docker socket.
- `/api/icon/<key>` — redirects to a linked service's real favicon.
- `/jellyfin/…`, `/navidrome/…`, `/filebrowser/…` — reverse-proxied straight
  through to each service on its internal port (what nginx used to do).

Glances (Sablier on-demand start) is not served through this app; it stays
disabled in `docker-compose.yml` for now.

## Local development

```bash
docker compose -f docker-compose.dev.yml up --build
```

Serves on http://localhost:8888 (mapped to the container's port 80).
Requires access to a Docker socket with the target containers running
(`self-hosted-streaming-*`) for `/api/usage` to return real data, and for the
proxy routes to reach jellyfin/navidrome/filebrowser you need to run this
alongside the full stack (shares its network namespace in production via
`network_mode: service:tailscale`).
