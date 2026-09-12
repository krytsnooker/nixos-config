# mc-control: minimal API letting the LAN homepage start/stop the Minecraft
# server on demand. Deliberately tiny — three endpoints, no framework
# extras. Runs as an unprivileged user; the ONLY elevated thing it can do is
# a narrowly-scoped sudo rule (see modules/minecraft.nix) allowing exactly
# `systemctl start|stop|is-active <the one minecraft unit>` — nothing else.
import os
import subprocess
from flask import Flask, jsonify

app = Flask(__name__)

SYSTEMCTL = "/run/current-system/sw/bin/systemctl"
SUDO = "/run/current-system/sw/bin/sudo"
MC_UNIT = os.environ["MC_UNIT"]  # set by systemd; fail loudly if missing


@app.after_request
def add_cors(resp):
    # Called cross-origin from the homepage (different port = different
    # origin per browser rules). LAN-only, no auth anywhere else on this
    # box's tools either, so an open CORS policy matches the existing
    # trust model rather than introducing a new one.
    resp.headers["Access-Control-Allow-Origin"] = "*"
    resp.headers["Access-Control-Allow-Methods"] = "GET, POST"
    return resp


def run_systemctl(action):
    try:
        result = subprocess.run(
            [SUDO, "-n", SYSTEMCTL, action, MC_UNIT],
            capture_output=True, text=True, timeout=15,
        )
        return result
    except subprocess.TimeoutExpired:
        return None


@app.route('/status')
def status():
    result = run_systemctl("is-active")
    if result is None:
        return jsonify({"status": "unknown", "error": "timeout"}), 504
    # is-active exits non-zero for inactive/failed states but still prints
    # the state to stdout — that's expected, not an error.
    state = result.stdout.strip() or "unknown"
    return jsonify({"status": state})


@app.route('/start', methods=['POST'])
def start():
    result = run_systemctl("start")
    if result is None:
        return jsonify({"ok": False, "error": "timeout"}), 504
    if result.returncode != 0:
        return jsonify({"ok": False, "error": result.stderr.strip()}), 500
    return jsonify({"ok": True})


@app.route('/stop', methods=['POST'])
def stop():
    result = run_systemctl("stop")
    if result is None:
        return jsonify({"ok": False, "error": "timeout"}), 504
    if result.returncode != 0:
        return jsonify({"ok": False, "error": result.stderr.strip()}), 500
    return jsonify({"ok": True})


if __name__ == '__main__':
    port = int(os.environ.get("MC_CONTROL_PORT", 5020))
    try:
        from waitress import serve
        serve(app, host='0.0.0.0', port=port)
    except ImportError:
        app.run(host='0.0.0.0', port=port, debug=False)
