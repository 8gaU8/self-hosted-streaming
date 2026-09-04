import os

import requests
from flask import Flask, abort, jsonify, redirect, render_template, request


app = Flask(__name__)

SERVICES = [
    {"name": "Navidrome", "port": 4533, "check_path": "/ping"},
    {"name": "File Browser", "port": 8080, "check_path": "/health"},
    {"name": "Jellyfin", "port": 8096, "check_path": "/health"},
]

# Path -> port redirects, migrated from landing/conf/nginx.conf
REDIRECTS = {
    "glances": 61208,
}


def check_service(port, check_path):
    try:
        resp = requests.get(f"http://127.0.0.1:{port}{check_path}", timeout=2)
        return resp.status_code < 500
    except requests.RequestException:
        return False


@app.route("/")
def index():
    return render_template("index.html", services=SERVICES)


@app.route("/api/status")
def status():
    results = []
    for svc in SERVICES:
        up = check_service(svc["port"], svc["check_path"])
        results.append({"name": svc["name"], "port": svc["port"], "up": up})
    return jsonify(results)


# @app.route("/api/metrics/cpu")
# def metrics():
#     # Placeholder for metrics endpoint
#     # return cpu metrics in JSON format
#     ...


@app.route("/<service>")
def redirect_service(service):
    port = REDIRECTS.get(service)
    if port is None:
        abort(404)
    host = request.host.split(":")[0]
    return redirect(f"{request.scheme}://{host}:{port}/")


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=81, debug=os.environ.get("FLASK_DEBUG") == "1")
