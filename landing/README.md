# landing

Go dashboard/proxy for the media server, and this stack's Tailscale entry
point. It joins the tailnet directly via
[`tsnet`](https://pkg.go.dev/tailscale.com/tsnet) (a Tailscale node embedded
in the binary, no separate `tailscale` sidecar container or `/dev/net/tun`
device needed) and is the sole entry point on the tailnet.

- `/` — dashboard: icon links to Jellyfin/Navidrome/File Browser, live
  status dots, and per-container + stack-wide CPU/memory/disk I/O usage.
- `/api/status` — JSON health check for the linked services.
- `/api/usage` — JSON CPU/memory/disk I/O usage per service plus a stack-wide
  total, read from the Docker socket.
- `/api/icon/<key>` — redirects to a linked service's real favicon.
- `/jellyfin/…`, `/navidrome/…`, `/filebrowser/…` — reverse-proxied straight
  through to each service over the docker-compose network.
- Direct TCP passthrough on Navidrome (4533), File Browser (8080), and
  Jellyfin (8096)'s own ports on the tailnet, for native apps that connect
  straight to a service instead of going through the `/<key>/` proxy.

## Local development

Without Docker, joining a tailnet at all:

```bash
go run .
```

`TS_AUTHKEY` unset means `landing` skips `tsnet` entirely and just listens
on plain HTTP (`:80`, or `$PORT` if set) -- same as the old Flask dev
server, and no Tailscale identity is created for local iteration.
`/api/usage` needs a reachable Docker socket to return real data, and the
`/<key>/` proxy routes need the rest of the stack (`jellyfin`, `navidrome`,
`filebrowser`) reachable by those container DNS names, so for a full
end-to-end check run the whole stack instead:

```bash
docker compose -f docker-compose.dev.yml up --build
```

Serves on http://localhost:8888 (mapped to the container's port 80), with
the host's Docker socket mounted read-only.

Production (`Dockerfile`) is the same binary; the only thing that changes
its behavior is whether `TS_AUTHKEY` is set (see the root
[`docker-compose.yml`](../docker-compose.yml)) -- if it is, `landing` joins
the tailnet via `tsnet` and serves from there instead of a plain listener.
`TS_STATE_DIR` (default `/var/lib/tsnet`) controls where the node's
Tailscale state is persisted across restarts.

## Migrating from the Flask version

This replaces the earlier Flask/gunicorn app (see git history) and the
standalone `tailscale` sidecar container:

- No more `tailscale` service, `ts-config/serve.json`, `/dev/net/tun`
  device, or `NET_ADMIN`/`NET_RAW` capabilities -- `tsnet` runs a userspace
  network stack, so none of that plumbing is needed anymore.
- `navidrome`/`filebrowser`/`jellyfin` no longer use
  `network_mode: service:tailscale`; they're on the default compose network
  and reached by container DNS name (`http://navidrome:4533`, etc.)
  instead of `127.0.0.1:<port>` in a shared network namespace.
- The direct-port passthrough that `ts-config/serve.json` used to provide
  (native apps connecting straight to Navidrome/Jellyfin/File Browser's own
  port over the tailnet) is preserved -- `landing` now does that itself via
  extra `tsnet` listeners instead of `tailscale serve`'s TCP forwarding.
- The dashboard used to also show a "Tailscale" card with that container's
  own resource usage; there's no longer a separate container for it, so
  that card is gone. Its usage is still folded into the "全体" total, the
  same as `landing`'s own usage always was.
