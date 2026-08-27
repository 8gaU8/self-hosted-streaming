import requests
from flask import Flask, jsonify, render_template

app = Flask(__name__)

SERVICES = [
    {"name": "Navidrome", "port": 4533, "check_path": "/ping"},
    {"name": "File Browser", "port": 8080, "check_path": "/health"},
    {"name": "Jellyfin", "port": 8096, "check_path": "/health"},
]


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


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=80)