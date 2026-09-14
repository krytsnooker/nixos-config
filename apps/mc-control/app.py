import json
import os
import subprocess
from flask import Flask, jsonify, request, render_template

_here = os.path.dirname(os.path.abspath(__file__))
app = Flask(__name__, template_folder=os.path.join(_here, "templates"))

SYSTEMCTL    = "/run/current-system/sw/bin/systemctl"
SUDO         = "/run/wrappers/bin/sudo"
CAT          = "/run/current-system/sw/bin/cat"
TEE          = "/run/current-system/sw/bin/tee"
MC_UNIT      = os.environ["MC_UNIT"]
MC_DATA_DIR  = os.environ.get("MC_DATA_DIR",  "/var/lib/minecraft/survival")
MC_STATE_DIR = os.environ.get("MC_STATE_DIR", "/var/lib/mc-control")
PROPS_FILE   = os.path.join(MC_DATA_DIR,  "server.properties")
SETTINGS_FILE = os.path.join(MC_STATE_DIR, "settings.json")

# Settings exposed in the UI: key -> (type, default)
EDITABLE = {
    "motd":             ("str",    "Home LAN Server"),
    "max-players":      ("int",    "20"),
    "difficulty":       ("choice", "easy"),
    "gamemode":         ("choice", "survival"),
    "white-list":       ("bool",   "false"),
    "pvp":              ("bool",   "true"),
    "view-distance":    ("int",    "10"),
    "spawn-protection": ("int",    "16"),
}
READONLY_KEYS   = ["online-mode", "server-port"]
DIFFICULTY_OPTS = {"peaceful", "easy", "normal", "hard"}
GAMEMODE_OPTS   = {"survival", "creative", "adventure", "spectator"}


@app.after_request
def add_cors(resp):
    resp.headers["Access-Control-Allow-Origin"]  = "*"
    resp.headers["Access-Control-Allow-Methods"] = "GET, POST"
    resp.headers["Access-Control-Allow-Headers"] = "Content-Type"
    return resp


def _run(cmd, *, stdin=None, timeout=15):
    try:
        return subprocess.run(cmd, input=stdin, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        return None


def _systemctl(action, timeout=15):
    return _run([SUDO, "-n", SYSTEMCTL, action, MC_UNIT], timeout=timeout)


def _load_settings():
    """Read user settings from the persistent JSON file."""
    try:
        with open(SETTINGS_FILE) as f:
            return json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        return {}


def _save_settings(updates):
    """Merge updates into settings.json."""
    current = _load_settings()
    current.update(updates)
    os.makedirs(MC_STATE_DIR, exist_ok=True)
    with open(SETTINGS_FILE, "w") as f:
        json.dump(current, f, indent=2)


def _read_props():
    r = _run([SUDO, "-n", CAT, PROPS_FILE])
    if r is None:
        return None, "timeout"
    if r.returncode != 0:
        return None, r.stderr.strip() or "read failed"
    props = {}
    for line in r.stdout.splitlines():
        line = line.strip()
        if line and not line.startswith("#"):
            key, _, val = line.partition("=")
            props[key.strip()] = val.strip()
    return props, None


def _write_props(updates):
    """Merge updates into server.properties, creating it if it doesn't exist."""
    r = _run([SUDO, "-n", CAT, PROPS_FILE])
    existing_lines = r.stdout.splitlines() if (r and r.returncode == 0) else []

    applied = set()
    out = []
    for line in existing_lines:
        s = line.rstrip()
        if s and not s.startswith("#"):
            key = s.split("=")[0].strip()
            if key in updates:
                out.append(f"{key}={updates[key]}")
                applied.add(key)
                continue
        out.append(s)
    for key, val in updates.items():
        if key not in applied:
            out.append(f"{key}={val}")

    w = _run([SUDO, "-n", TEE, PROPS_FILE], stdin="\n".join(out) + "\n", timeout=5)
    if w is None:
        return "timeout writing file"
    if w.returncode != 0:
        return w.stderr.strip() or "write failed"
    return None


def _validate(data):
    updates = {}
    for key, (typ, _) in EDITABLE.items():
        if key not in data:
            continue
        val = str(data[key]).strip()
        if typ == "int":
            try:
                int(val)
            except ValueError:
                return None, f"invalid integer for '{key}'"
        elif typ == "bool":
            if val.lower() not in ("true", "false"):
                return None, f"'{key}' must be true or false"
            val = val.lower()
        elif typ == "choice":
            opts = DIFFICULTY_OPTS if key == "difficulty" else GAMEMODE_OPTS
            if val.lower() not in opts:
                return None, f"invalid value '{val}' for '{key}'"
            val = val.lower()
        updates[key] = val
    return updates, None


# ── Routes ───────────────────────────────────────────────────────────────────

@app.route("/")
def index():
    return render_template("index.html")


@app.route("/status")
def status():
    r = _systemctl("is-active")
    if r is None:
        return jsonify({"status": "unknown", "error": "timeout"}), 504
    return jsonify({"status": r.stdout.strip() or "unknown"})


@app.route("/start", methods=["POST"])
def start():
    # Apply saved settings to server.properties before starting so a
    # nixos-rebuild (which resets server.properties) never loses user config.
    saved = _load_settings()
    if saved:
        err = _write_props(saved)
        if err:
            return jsonify({"ok": False, "error": "failed to apply settings: " + err}), 500

    r = _systemctl("start")
    if r is None:
        return jsonify({"ok": False, "error": "timeout"}), 504
    if r.returncode != 0:
        return jsonify({"ok": False, "error": r.stderr.strip()}), 500
    return jsonify({"ok": True})


@app.route("/stop", methods=["POST"])
def stop():
    r = _systemctl("stop", timeout=90)
    if r is None:
        return jsonify({"ok": False, "error": "timeout"}), 504
    if r.returncode != 0:
        return jsonify({"ok": False, "error": r.stderr.strip()}), 500
    return jsonify({"ok": True})


@app.route("/config")
def get_config():
    saved = _load_settings()
    props, _ = _read_props()

    settings = {}
    for key, (_, default) in EDITABLE.items():
        if key in saved:
            settings[key] = saved[key]
        elif props and key in props:
            settings[key] = props[key]
        else:
            settings[key] = default

    readonly = {k: (props.get(k, "—") if props else "—") for k in READONLY_KEYS}
    return jsonify({"settings": settings, "readonly": readonly, "file_exists": props is not None})


@app.route("/config", methods=["POST"])
def post_config():
    data = request.get_json(force=True) or {}
    updates, err = _validate(data.get("settings", data))
    if err:
        return jsonify({"ok": False, "error": err}), 400

    _save_settings(updates)

    err = _write_props(updates)
    if err:
        return jsonify({"ok": False, "error": err}), 500
    return jsonify({"ok": True})


@app.route("/config/restart", methods=["POST"])
def config_restart():
    """Save settings then stop -> write -> start."""
    data = request.get_json(force=True) or {}
    updates, err = _validate(data.get("settings", data))
    if err:
        return jsonify({"ok": False, "error": err}), 400

    sr = _systemctl("is-active")
    was_running = sr and sr.stdout.strip() == "active"

    if was_running:
        r = _systemctl("stop", timeout=90)
        if r is None:
            return jsonify({"ok": False, "error": "stop timed out"}), 504
        if r.returncode != 0:
            return jsonify({"ok": False, "error": "stop failed: " + r.stderr.strip()}), 500

    _save_settings(updates)

    err = _write_props(updates)
    if err:
        return jsonify({"ok": False, "error": err}), 500

    r = _systemctl("start")
    if r is None:
        return jsonify({"ok": False, "error": "start timed out"}), 504
    if r.returncode != 0:
        return jsonify({"ok": False, "error": "start failed: " + r.stderr.strip()}), 500

    return jsonify({"ok": True, "restarted": True})


if __name__ == "__main__":
    port = int(os.environ.get("MC_CONTROL_PORT", 5020))
    try:
        from waitress import serve
        serve(app, host="0.0.0.0", port=port)
    except ImportError:
        app.run(host="0.0.0.0", port=port, debug=False)
