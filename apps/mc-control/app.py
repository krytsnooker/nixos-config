import json
import os
import socket
import struct
import subprocess
import urllib.parse
import urllib.request
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
MC_PORT      = int(os.environ.get("MC_PORT",  "25565"))
PROPS_FILE   = os.path.join(MC_DATA_DIR,  "server.properties")
SETTINGS_FILE = os.path.join(MC_STATE_DIR, "settings.json")
MODS_DIR         = os.path.join(MC_DATA_DIR,  "mods")
CF_KEY_FILE      = os.path.join(MC_STATE_DIR, "curseforge.env")
MAX_PLUGIN_SIZE  = 50 * 1024 * 1024  # 50 MB
CF_GAME_ID       = 432   # Minecraft
CF_MC_VERSION    = "1.20.1"
CF_MC_FAMILY     = ".".join(CF_MC_VERSION.split(".")[:2])   # "1.19"

DEFAULT_ALLOWED_HOSTS = [
    "cdn.modrinth.com",
    "hangar.papermc.io",
    "github.com",
    "objects.githubusercontent.com",
    "mediafilez.forgecdn.net",
    "edge.forgecdn.net",
]

# key -> (type, default)
EDITABLE = {
    # World
    "level-name":                    ("str",    "world"),
    "level-seed":                    ("str",    ""),
    "level-type":                    ("choice", "minecraft:normal"),
    "generate-structures":           ("bool",   "true"),
    "allow-nether":                  ("bool",   "true"),
    # Gameplay
    "gamemode":                      ("choice", "survival"),
    "difficulty":                    ("choice", "easy"),
    "force-gamemode":                ("bool",   "false"),
    "hardcore":                      ("bool",   "false"),
    "pvp":                           ("bool",   "true"),
    "allow-flight":                  ("bool",   "false"),
    "spawn-animals":                 ("bool",   "true"),
    "spawn-monsters":                ("bool",   "true"),
    "spawn-npcs":                    ("bool",   "true"),
    "enable-command-block":          ("bool",   "false"),
    # Players
    "motd":                          ("str",    "Home LAN Server"),
    "max-players":                   ("int",    "20"),
    "player-idle-timeout":           ("int",    "0"),
    "white-list":                    ("bool",   "false"),
    "enforce-whitelist":             ("bool",   "false"),
    "op-permission-level":           ("choice", "4"),
    # Rendering
    "view-distance":                 ("int",    "10"),
    "simulation-distance":           ("int",    "10"),
    "spawn-protection":              ("int",    "16"),
    "max-world-size":                ("int",    "29999984"),
    # Performance
    "network-compression-threshold": ("int",    "256"),
    "max-tick-time":                 ("int",    "60000"),
    "sync-chunk-writes":             ("bool",   "true"),
    "use-native-transport":          ("bool",   "true"),
    # Resource pack
    "resource-pack":                 ("str",    ""),
    "resource-pack-sha1":            ("str",    ""),
    "resource-pack-prompt":          ("str",    ""),
    "require-resource-pack":         ("bool",   "false"),
}

READONLY_KEYS = ["online-mode", "server-port"]

CHOICES = {
    "difficulty":          {"peaceful", "easy", "normal", "hard"},
    "gamemode":            {"survival", "creative", "adventure", "spectator"},
    "level-type":          {"minecraft:normal", "minecraft:flat", "minecraft:large_biomes",
                            "minecraft:amplified", "default", "flat", "largebiomes", "amplified"},
    "op-permission-level": {"1", "2", "3", "4"},
}


@app.after_request
def add_cors(resp):
    resp.headers["Access-Control-Allow-Origin"]  = "*"
    resp.headers["Access-Control-Allow-Methods"] = "GET, POST, DELETE"
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
    try:
        with open(SETTINGS_FILE) as f:
            return json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        return {}


def _save_settings(updates):
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
            opts = CHOICES.get(key, set())
            if val.lower() not in opts and val not in opts:
                return None, f"invalid value '{val}' for '{key}'"
        updates[key] = val
    return updates, None


def _get_lan_ip():
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        return ip
    except Exception:
        return None


def _ping_server(port, timeout=3):
    try:
        sock = socket.create_connection(("127.0.0.1", port), timeout=timeout)
        sock.settimeout(timeout)

        def varint(n):
            out = b""
            while True:
                b = n & 0x7F
                n >>= 7
                out += bytes([b | (0x80 if n else 0)])
                if not n:
                    break
            return out

        def read_bytes(n):
            buf = b""
            while len(buf) < n:
                chunk = sock.recv(n - len(buf))
                if not chunk:
                    raise EOFError
                buf += chunk
            return buf

        def read_varint():
            n = shift = 0
            while True:
                b = read_bytes(1)[0]
                n |= (b & 0x7F) << shift
                if not (b & 0x80):
                    return n
                shift += 7

        def send_pkt(pid, data=b""):
            body = varint(pid) + data
            sock.sendall(varint(len(body)) + body)

        def mc_string(s):
            enc = s.encode("utf-8")
            return varint(len(enc)) + enc

        send_pkt(0x00,
            varint(762) + mc_string("127.0.0.1") + struct.pack(">H", port) + varint(1))
        send_pkt(0x00)

        read_varint()
        read_varint()
        jdata = read_bytes(read_varint())
        sock.close()
        return json.loads(jdata)
    except Exception:
        return None


def _get_uptime():
    r = _run([SYSTEMCTL, "show", MC_UNIT,
              "--property=ActiveEnterTimestampMonotonic",
              "--property=ActiveState"])
    if not r or r.returncode != 0:
        return None
    props = {}
    for line in r.stdout.strip().splitlines():
        k, _, v = line.partition("=")
        props[k.strip()] = v.strip()
    if props.get("ActiveState") != "active":
        return None
    try:
        mono_us = int(props.get("ActiveEnterTimestampMonotonic", "0"))
        if mono_us == 0:
            return None
        with open("/proc/uptime") as f:
            boot_s = float(f.read().split()[0])
        elapsed = max(0, boot_s - mono_us / 1_000_000)
        h, rem = divmod(int(elapsed), 3600)
        m, s   = divmod(rem, 60)
        if h:   return f"{h}h {m}m"
        if m:   return f"{m}m {s}s"
        return f"{s}s"
    except Exception:
        return None


def _get_allowed_hosts():
    s = _load_settings()
    if "allowed_hosts" not in s:
        return list(DEFAULT_ALLOWED_HOSTS)
    return s["allowed_hosts"]


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


@app.route("/info")
def get_info():
    lan_ip = _get_lan_ip()
    props, _ = _read_props()
    port  = int(props["server-port"]) if props and "server-port" in props else MC_PORT
    world = props.get("level-name", "world") if props else "world"

    address = f"{lan_ip}:{port}" if lan_ip else f"homeserver:{port}"
    uptime  = _get_uptime()
    ping    = _ping_server(port) if uptime else None

    players = None
    if ping:
        p = ping.get("players", {})
        players = {
            "online": p.get("online", 0),
            "max":    p.get("max", 20),
            "names":  [e["name"] for e in p.get("sample", [])],
        }

    return jsonify({"address": address, "world": world, "uptime": uptime, "players": players})


@app.route("/start", methods=["POST"])
def start():
    saved = _load_settings()
    if saved:
        err = _write_props({k: v for k, v in saved.items() if k in EDITABLE})
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


# ── Plugin routes ─────────────────────────────────────────────────────────────

@app.route("/plugins")
def list_plugins():
    try:
        files = sorted(f for f in os.listdir(MODS_DIR) if f.lower().endswith(".jar"))
    except FileNotFoundError:
        files = []
    return jsonify({"plugins": files})


@app.route("/plugins/search")
def search_plugins():
    q = request.args.get("q", "").strip()
    if not q:
        return jsonify({"results": []})
    params = urllib.parse.urlencode({
        "query": q,
        "facets": json.dumps([
            ["categories:neoforge", "categories:forge"],
            ["versions:1.20.1"],
        ]),
        "limit": "10",
        "index": "downloads",
    })
    url = f"https://api.modrinth.com/v2/search?{params}"
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "mc-control/1.0"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            data = json.loads(resp.read())
        results = []
        for hit in data.get("hits", []):
            slug = hit.get("slug", hit["project_id"])
            results.append({
                "id":          hit["project_id"],
                "slug":        slug,
                "title":       hit["title"],
                "description": hit.get("description", ""),
                "downloads":   hit.get("downloads", 0),
                "icon_url":    hit.get("icon_url"),
                "website_url": f"https://modrinth.com/project/{slug}",
            })
        return jsonify({"results": results})
    except Exception as e:
        return jsonify({"results": [], "error": str(e)})


@app.route("/plugins/resolve/<project_id>")
def resolve_plugin(project_id):
    if not project_id or "/" in project_id or ".." in project_id:
        return jsonify({"ok": False, "error": "invalid project id"}), 400
    params = urllib.parse.urlencode({
        "loaders":       json.dumps(["neoforge", "forge"]),
        "game_versions": json.dumps(["1.20.1"]),
    })
    url = f"https://api.modrinth.com/v2/project/{project_id}/version?{params}"
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "mc-control/1.0"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            versions = json.loads(resp.read())
        if not versions:
            return jsonify({"ok": False, "error": "no compatible version found for Paper 1.19.4"}), 404
        v = versions[0]
        files = [f for f in v.get("files", []) if f.get("primary")]
        if not files:
            files = v.get("files", [])
        if not files:
            return jsonify({"ok": False, "error": "no files in version"}), 404
        return jsonify({
            "ok":       True,
            "version":  v["version_number"],
            "url":      files[0]["url"],
            "filename": files[0]["filename"],
        })
    except Exception as e:
        return jsonify({"ok": False, "error": str(e)}), 500


@app.route("/plugins/fetch", methods=["POST"])
def fetch_plugin():
    data = request.get_json(force=True) or {}
    url = data.get("url", "").strip()
    if not url:
        return jsonify({"ok": False, "error": "no URL provided"}), 400

    try:
        parsed = urllib.parse.urlparse(url)
    except Exception:
        return jsonify({"ok": False, "error": "invalid URL"}), 400

    allowed = _get_allowed_hosts()
    if parsed.hostname not in allowed:
        return jsonify({"ok": False, "blocked_host": parsed.hostname,
                        "error": f"host '{parsed.hostname}' is not in the allowed list"}), 403

    filename = os.path.basename(parsed.path.rstrip("/"))
    if not filename.lower().endswith(".jar"):
        return jsonify({"ok": False, "error": "only .jar files are allowed"}), 400
    if not filename or ".." in filename or "/" in filename:
        return jsonify({"ok": False, "error": "invalid filename"}), 400

    try:
        os.makedirs(MODS_DIR, exist_ok=True)
    except OSError as e:
        return jsonify({"ok": False, "error": f"cannot access plugins directory: {e}"}), 500

    dest = os.path.join(MODS_DIR, filename)
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "mc-control/1.0"})
        with urllib.request.urlopen(req, timeout=30) as resp:
            size = 0
            with open(dest, "wb") as f:
                while True:
                    chunk = resp.read(65536)
                    if not chunk:
                        break
                    size += len(chunk)
                    if size > MAX_PLUGIN_SIZE:
                        f.close()
                        try: os.unlink(dest)
                        except OSError: pass
                        return jsonify({"ok": False, "error": "file exceeds 50 MB limit"}), 400
                    f.write(chunk)
    except Exception as e:
        try: os.unlink(dest)
        except OSError: pass
        return jsonify({"ok": False, "error": str(e)}), 500

    return jsonify({"ok": True, "filename": filename})


@app.route("/plugins/<name>", methods=["DELETE"])
def delete_plugin(name):
    if not name.lower().endswith(".jar") or "/" in name or ".." in name:
        return jsonify({"ok": False, "error": "invalid filename"}), 400
    path = os.path.join(MODS_DIR, name)
    try:
        os.unlink(path)
    except FileNotFoundError:
        return jsonify({"ok": False, "error": "not found"}), 404
    except OSError as e:
        return jsonify({"ok": False, "error": str(e)}), 500
    return jsonify({"ok": True})


@app.route("/plugins/allowed-hosts")
def get_allowed_hosts_route():
    return jsonify({"hosts": _get_allowed_hosts()})


@app.route("/plugins/allowed-hosts", methods=["POST"])
def post_allowed_hosts():
    data = request.get_json(force=True) or {}
    hosts = data.get("hosts", [])
    if not isinstance(hosts, list):
        return jsonify({"ok": False, "error": "hosts must be a list"}), 400
    cleaned = []
    for h in hosts:
        if not isinstance(h, str):
            return jsonify({"ok": False, "error": "invalid host entry"}), 400
        h = h.strip().lower()
        if h:
            cleaned.append(h)
    _save_settings({"allowed_hosts": cleaned})
    return jsonify({"ok": True})


# ── CurseForge routes ─────────────────────────────────────────────────────────

def _get_cf_key():
    try:
        with open(CF_KEY_FILE) as f:
            for line in f:
                line = line.strip()
                if line.startswith("CURSEFORGE_API_KEY="):
                    return line.split("=", 1)[1].strip()
    except FileNotFoundError:
        pass
    return None


def _cf_request(path, params=None):
    key = _get_cf_key()
    if not key:
        raise RuntimeError("CurseForge API key not configured")
    url = "https://api.curseforge.com" + path
    if params:
        url += "?" + urllib.parse.urlencode(params)
    req = urllib.request.Request(url, headers={
        "x-api-key": key,
        "Accept": "application/json",
        "User-Agent": "mc-control/1.0",
    })
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.loads(resp.read())


@app.route("/plugins/cf-search")
def cf_search():
    q = request.args.get("q", "").strip()
    if not q:
        return jsonify({"results": []})
    try:
        data = _cf_request("/v1/mods/search", {
            "gameId":       CF_GAME_ID,
            "searchFilter": q,
            "gameVersion":  CF_MC_VERSION,
            "sortField":    "6",   # TotalDownloads
            "sortOrder":    "desc",
            "pageSize":     "10",
        })
        results = []
        for mod in data.get("data", []):
            results.append({
                "id":          mod["id"],
                "title":       mod["name"],
                "description": mod.get("summary", ""),
                "downloads":   mod.get("downloadCount", 0),
                "icon_url":    (mod.get("logo") or {}).get("url"),
                "website_url": (mod.get("links") or {}).get("websiteUrl"),
            })
        return jsonify({"results": results})
    except RuntimeError as e:
        return jsonify({"results": [], "error": str(e), "no_key": True})
    except Exception as e:
        return jsonify({"results": [], "error": str(e)})


@app.route("/plugins/cf-resolve/<int:mod_id>")
def cf_resolve(mod_id):
    try:
        data = _cf_request(f"/v1/mods/{mod_id}")
        mod = data.get("data", {})

        # Build lookup of fileId -> file from latestFiles
        all_files = {f["id"]: f for f in mod.get("latestFiles", [])}

        # Walk latestFilesIndexes collecting the best match: exact version first,
        # then family (e.g. "1.19" for a "1.19.4" server).
        target_id = None
        family_id = None
        for idx in mod.get("latestFilesIndexes", []):
            gv = idx.get("gameVersion", "")
            if gv == CF_MC_VERSION:
                target_id = idx.get("fileId")
                break
            if gv == CF_MC_FAMILY and not family_id:
                family_id = idx.get("fileId")

        chosen = None

        # 1. Index exact match — latestFiles may be truncated; fetch file directly if missing
        if target_id:
            candidate = all_files.get(target_id)
            if candidate is None:
                try:
                    fd = _cf_request(f"/v1/mods/{mod_id}/files/{target_id}")
                    candidate = fd.get("data")
                except Exception:
                    pass
            if candidate:
                chosen = candidate

        # 2. latestFiles exact match
        if not chosen:
            exact = [lf for lf in all_files.values() if CF_MC_VERSION in lf.get("gameVersions", [])]
            if exact:
                chosen = sorted(exact, key=lambda x: x.get("id", 0), reverse=True)[0]

        # 3. Index family match ("1.21" covers "1.21.1" on CurseForge)
        if not chosen and family_id:
            candidate = all_files.get(family_id)
            if candidate is None:
                try:
                    fd = _cf_request(f"/v1/mods/{mod_id}/files/{family_id}")
                    candidate = fd.get("data")
                except Exception:
                    pass
            if candidate:
                chosen = candidate

        # 4. latestFiles family match
        if not chosen:
            family = [lf for lf in all_files.values() if CF_MC_FAMILY in lf.get("gameVersions", [])]
            if family:
                chosen = sorted(family, key=lambda x: x.get("id", 0), reverse=True)[0]

        # 5. Files API query — last resort when latestFiles/indexes gave nothing
        if not chosen:
            try:
                fdata = _cf_request(f"/v1/mods/{mod_id}/files", {
                    "gameVersion": CF_MC_VERSION,
                    "pageSize":    "10",
                })
                files = fdata.get("data", [])
                if files:
                    chosen = sorted(files, key=lambda x: x.get("id", 0), reverse=True)[0]
            except Exception:
                pass

        if not chosen:
            versions = sorted({
                v for lf in all_files.values()
                for v in lf.get("gameVersions", [])
                if v.startswith("1.")
            })
            return jsonify({
                "ok": False,
                "error": f"No file compatible with {CF_MC_VERSION}. "
                         f"Available MC versions: {', '.join(versions) or 'unknown'}",
            }), 404

        f = chosen

        dl_url = f.get("downloadUrl")
        if not dl_url:
            return jsonify({"ok": False, "error": "Author has disabled third-party downloads for this mod"}), 403

        return jsonify({
            "ok":       True,
            "version":  f.get("displayName", str(f["id"])),
            "url":      dl_url,
            "filename": f["fileName"],
        })
    except RuntimeError as e:
        return jsonify({"ok": False, "error": str(e), "no_key": True}), 503
    except Exception as e:
        return jsonify({"ok": False, "error": str(e)}), 500


if __name__ == "__main__":
    port = int(os.environ.get("MC_CONTROL_PORT", 5020))
    try:
        from waitress import serve
        serve(app, host="0.0.0.0", port=port)
    except ImportError:
        app.run(host="0.0.0.0", port=port, debug=False)
