import os
import re
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from urllib.parse import urljoin

import docker
import requests
from flask import (
    Flask,
    Response,
    abort,
    jsonify,
    redirect,
    render_template,
    request,
    stream_with_context,
)

app = Flask(__name__)

docker_client = docker.from_env()

# Services with a web UI. They're reverse-proxied by nginx at `link`, and
# reachable directly at 127.0.0.1:<port> for server-side health checks since
# every container shares the tailscale container's network namespace.
#
# `icon_page` is the same app fetched directly (bypassing nginx, same as the
# health check) so /api/icon/<key> can scrape its real <link rel="icon">
# instead of hardcoding a filename that changes across releases.
LINKED_SERVICES = [
    {
        "key": "jellyfin",
        "name": "Jellyfin",
        "port": 8096,
        "check_path": "/health",
        "link": "/jellyfin/",
        "icon_page": "/jellyfin/web/",
    },
    {
        "key": "navidrome",
        "name": "Navidrome",
        "port": 4533,
        "check_path": "/ping",
        "link": "/navidrome/",
        "icon_page": "/navidrome/app/",
    },
    {
        "key": "filebrowser",
        "name": "File Browser",
        "port": 8080,
        "check_path": "/health",
        "link": "/filebrowser/",
        "icon_page": "/filebrowser/",
    },
]

_ICON_LINK_RE = re.compile(
    r'<link[^>]+rel=["\'](?:shortcut icon|icon)["\'][^>]*href=["\']([^"\']+)["\']',
    re.IGNORECASE,
)
_icon_url_cache = {}

# Containers whose resource usage is shown on the dashboard as its own card.
# Tailscale has no web UI of its own, so it's monitored but not linked.
MONITORED_SERVICES = LINKED_SERVICES + [
    {"key": "tailscale", "name": "Tailscale", "link": None},
]


def check_service(port, check_path):
    try:
        resp = requests.get(f"http://127.0.0.1:{port}{check_path}", timeout=2)
        return resp.status_code < 500
    except requests.RequestException:
        return False


def find_container(compose_service):
    containers = docker_client.containers.list(
        filters={"label": f"com.docker.compose.service={compose_service}"}
    )
    return containers[0] if containers else None


def list_project_containers():
    """All running containers belonging to this docker-compose project."""
    for svc in MONITORED_SERVICES:
        container = find_container(svc["key"])
        project = container.labels.get("com.docker.compose.project") if container else None
        if project:
            return docker_client.containers.list(filters={"label": f"com.docker.compose.project={project}"})
    return []


def cpu_percent(stats):
    cpu_stats = stats.get("cpu_stats", {})
    precpu_stats = stats.get("precpu_stats", {})

    cpu_usage = cpu_stats.get("cpu_usage", {})
    precpu_usage = precpu_stats.get("cpu_usage", {})

    cpu_delta = cpu_usage.get("total_usage", 0) - precpu_usage.get("total_usage", 0)
    system_delta = cpu_stats.get("system_cpu_usage", 0) - precpu_stats.get("system_cpu_usage", 0)

    online_cpus = (
        cpu_stats.get("online_cpus")
        or len(cpu_usage.get("percpu_usage") or [])
        or os.cpu_count()
        or 1
    )

    if system_delta <= 0 or cpu_delta < 0:
        return 0.0

    return round((cpu_delta / system_delta) * online_cpus * 100, 1)


def memory_usage(stats):
    mem_stats = stats.get("memory_stats", {})
    usage = mem_stats.get("usage", 0)
    limit = mem_stats.get("limit", 0)

    # Match `docker stats`: exclude page cache so this reflects actual memory
    # pressure rather than the kernel's reclaimable disk cache.
    detail = mem_stats.get("stats", {})
    cache = detail.get("inactive_file", detail.get("cache", 0))
    usage = max(usage - cache, 0)

    return usage, limit


def blkio_bytes(stats):
    entries = stats.get("blkio_stats", {}).get("io_service_bytes_recursive") or []

    read_bytes = sum(e["value"] for e in entries if e.get("op", "").lower() == "read")
    write_bytes = sum(e["value"] for e in entries if e.get("op", "").lower() == "write")

    return read_bytes, write_bytes


def usage_from_stats(stats):
    mem_used, mem_limit = memory_usage(stats)
    disk_read, disk_write = blkio_bytes(stats)

    return {
        "cpu_percent": cpu_percent(stats),
        "mem_used": mem_used,
        "mem_limit": mem_limit,
        "mem_percent": round(mem_used / mem_limit * 100, 1) if mem_limit else None,
        "disk_read": disk_read,
        "disk_write": disk_write,
    }


def fetch_stats(container):
    try:
        return container, container.stats(stream=False)
    except docker.errors.APIError:
        return container, None


def resolve_icon_url(svc):
    if svc["key"] in _icon_url_cache:
        return _icon_url_cache[svc["key"]]

    try:
        resp = requests.get(f"http://127.0.0.1:{svc['port']}{svc['icon_page']}", timeout=3)
        resp.raise_for_status()
    except requests.RequestException:
        return None

    match = _ICON_LINK_RE.search(resp.text)
    if not match:
        return None

    # The href is relative to icon_page, which nginx proxies 1:1 (no path
    # rewriting), so resolving it against that path also gives the public URL.
    icon_url = urljoin(svc["icon_page"], match.group(1))
    _icon_url_cache[svc["key"]] = icon_url
    return icon_url


PROXY_METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"]

# Headers that only make sense hop-by-hop, or that Werkzeug already sets
# itself, and so must not be copied from the upstream response onto ours.
_EXCLUDED_RESPONSE_HEADERS = {
    "content-encoding",
    "content-length",
    "transfer-encoding",
    "connection",
    "date",
    "server",
}


def reverse_proxy(port):
    """Forward the current request to a service sharing our network
    namespace, streaming the response back. Replaces the equivalent
    `proxy_pass` blocks nginx used to run in front of this app."""
    target_url = f"http://127.0.0.1:{port}{request.full_path}"
    if target_url.endswith("?"):
        target_url = target_url[:-1]

    headers = {k: v for k, v in request.headers.items() if k.lower() not in ("host", "connection", "content-length")}
    headers["Host"] = request.host

    try:
        upstream = requests.request(
            method=request.method,
            url=target_url,
            headers=headers,
            data=request.get_data(),
            stream=True,
            allow_redirects=False,
            timeout=30,
        )
    except requests.RequestException:
        abort(502)

    response_headers = [(k, v) for k, v in upstream.headers.items() if k.lower() not in _EXCLUDED_RESPONSE_HEADERS]

    return Response(
        stream_with_context(upstream.iter_content(chunk_size=65536)),
        status=upstream.status_code,
        headers=response_headers,
    )


def register_proxy(key, port):
    def view(path=""):
        return reverse_proxy(port)

    view.__name__ = f"proxy_{key}"
    app.add_url_rule(f"/{key}/", defaults={"path": ""}, view_func=view, methods=PROXY_METHODS)
    app.add_url_rule(f"/{key}/<path:path>", view_func=view, methods=PROXY_METHODS)


for _svc in LINKED_SERVICES:
    register_proxy(_svc["key"], _svc["port"])


@app.route("/")
def index():
    return render_template("index.html", services=MONITORED_SERVICES)


@app.route("/api/icon/<key>")
def icon(key):
    svc = next((s for s in LINKED_SERVICES if s["key"] == key), None)
    if svc is None:
        abort(404)

    icon_url = resolve_icon_url(svc)
    if icon_url is None:
        abort(404)

    return redirect(icon_url)


@app.route("/api/status")
def status():
    results = [
        {"key": svc["key"], "name": svc["name"], "up": check_service(svc["port"], svc["check_path"])}
        for svc in LINKED_SERVICES
    ]
    return jsonify(results)


@app.route("/api/usage")
def usage():
    containers = list_project_containers()

    # The Docker Engine API takes ~1-2s per container to return a one-shot
    # stats sample (it has to observe two cgroup reads a beat apart to
    # compute deltas). Fetching sequentially made this endpoint take
    # N * ~1.5s; fetching in parallel bounds it to ~1.5s regardless of how
    # many containers are in the stack.
    with ThreadPoolExecutor(max_workers=max(len(containers), 1)) as pool:
        results = list(pool.map(fetch_stats, containers))

    by_service = {}
    total = {"cpu_percent": 0.0, "mem_used": 0, "mem_limit": 0, "disk_read": 0, "disk_write": 0, "container_count": 0}

    for container, stats in results:
        if stats is None:
            continue

        u = usage_from_stats(stats)
        by_service[container.labels.get("com.docker.compose.service", container.name)] = u

        total["cpu_percent"] += u["cpu_percent"]
        total["mem_used"] += u["mem_used"]
        total["mem_limit"] = max(total["mem_limit"], u["mem_limit"])
        total["disk_read"] += u["disk_read"]
        total["disk_write"] += u["disk_write"]
        total["container_count"] += 1

    total["cpu_percent"] = round(total["cpu_percent"], 1)
    total["mem_percent"] = round(total["mem_used"] / total["mem_limit"] * 100, 1) if total["mem_limit"] else None

    services = [
        {"key": svc["key"], "name": svc["name"], "available": svc["key"] in by_service, **by_service.get(svc["key"], {})}
        for svc in MONITORED_SERVICES
    ]

    return jsonify({
        "updated_at": datetime.now(timezone.utc).isoformat(),
        "services": services,
        "total": total,
    })


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=80, debug=os.environ.get("FLASK_DEBUG") == "1")
