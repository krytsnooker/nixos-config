import threading
from flask import Flask, render_template, jsonify, request

# Modular imports from our project files
from config import PORT, EMBY_EXTERNAL_URL
from database import init_db, query_db, get_where_used, get_db
from services import (
    get_combined_metadata,
    fetch_discogs_data,
    get_emby_artist_id,
    get_emby_server_id
)

app = Flask(__name__)

# Cache the Emby Server ID so we don't query it on every click. Guarded by a
# lock since the production server (waitress) is multi-threaded, unlike the
# old single-threaded Flask dev server this was written against.
CACHED_EMBY_SERVER_ID = None
_emby_server_id_lock = threading.Lock()


def get_cached_emby_server_id():
    global CACHED_EMBY_SERVER_ID
    if CACHED_EMBY_SERVER_ID is None:
        with _emby_server_id_lock:
            if CACHED_EMBY_SERVER_ID is None:
                CACHED_EMBY_SERVER_ID = get_emby_server_id()
    return CACHED_EMBY_SERVER_ID

# --- ROUTES ---

@app.route('/')
def index():
    """Renders the main page and populates the Artist dropdown."""
    artists = query_db("SELECT DISTINCT artist FROM tracks ORDER BY artist COLLATE NOCASE")
    return render_template('index.html', artists=artists)

@app.route('/get_albums/<artist>')
def get_albums(artist):
    """Returns a list of unique albums for a selected artist."""
    albums = query_db("SELECT DISTINCT album FROM tracks WHERE artist = ? ORDER BY album COLLATE NOCASE", [artist])
    return jsonify([a[0] for a in albums])

@app.route('/get_tracks/<artist>/<album>')
def get_tracks(artist, album):
    """Returns the tracklist for an album including durations."""
    query = "SELECT track_no, title, duration FROM tracks WHERE artist = ? AND album = ? ORDER BY CAST(track_no AS INTEGER)"
    tracks = query_db(query, [artist, album])
    return jsonify([{"no": t[0], "title": t[1], "duration": t[2]} for t in tracks])

@app.route('/get_mb_info/<artist>/<album>')
def get_mb_info(artist, album):
    """
    Main data orchestrator. 
    1. Handles Emby Deep Linking (with ServerID fix)
    2. Handles MusicBrainz/Wiki Metadata Caching
    3. Handles Discogs Relational Credits
    """
    # 1. EMBY INTEGRATION
    emby_server_id = get_cached_emby_server_id()

    # Get the specific Artist ID
    eid = get_emby_artist_id(artist)

    # Construct the full absolute URL with the serverId parameter to prevent black screens
    emby_link = None
    if eid and emby_server_id:
        emby_link = f"{EMBY_EXTERNAL_URL}/web/index.html#!/item?id={eid}&serverId={emby_server_id}"

    # 2. METADATA CACHE (MB & Wiki)
    res = query_db("""
        SELECT year, country, label, mbid, artist_summary, album_summary 
        FROM album_metadata 
        WHERE artist = ? AND album = ?
    """, [artist, album], one=True)
    
    # If Metadata is missing or bio is empty, fetch fresh data
    if not res or not res[4]:
        meta = get_combined_metadata(artist, album)
        if meta:
            with get_db() as conn:
                conn.execute("""
                    INSERT OR REPLACE INTO album_metadata
                    (artist, album, year, country, label, mbid, artist_summary, album_summary)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                """, (artist, album, meta['year'], meta['country'], meta['label'],
                      meta['mbid'], meta['artist_summary'], meta['album_summary']))
                conn.commit()
            res = (meta['year'], meta['country'], meta['label'], meta['mbid'], meta['artist_summary'], meta['album_summary'])

    # 3. DISCOGS INTEGRATION (Credits & Companies)
    credits_rows = query_db("SELECT entity_name, role FROM album_credits WHERE artist = ? AND album = ?", [artist, album])
    
    if not credits_rows:
        fetch_discogs_data(artist, album)
        credits_rows = query_db("SELECT entity_name, role FROM album_credits WHERE artist = ? AND album = ?", [artist, album])
    
    companies_rows = query_db("SELECT entity_name, entity_type FROM album_companies WHERE artist = ? AND album = ?", [artist, album])

    # 4. JSON RESPONSE
    return jsonify({
        "year": res[0] if res else "Unknown",
        "country": res[1] if res else "Unknown",
        "label": res[2] if res else "Unknown",
        "mbid": res[3] if res else None,
        "artist_summary": res[4] if res else "",
        "album_summary": res[5] if res else "",
        "credits": [{"name": c[0], "role": c[1]} for c in credits_rows],
        "companies": [{"name": co[0], "type": co[1]} for co in companies_rows],
        "emby_link": emby_link
    })

@app.route('/where_used/<path:entity_name>')
def where_used_route(entity_name):
    """API for the 'Where Used' feature."""
    results = get_where_used(entity_name)
    return jsonify([{"artist": r[0], "album": r[1], "role": r[2]} for r in results])

# --- MAIN ---

if __name__ == '__main__':
    # Initialize database tables
    init_db()

    try:
        from waitress import serve
        print(f"--- Server starting (waitress) on port {PORT} ---")
        serve(app, host='0.0.0.0', port=PORT)
    except ImportError:
        print(f"--- Server starting on port {PORT} (waitress not installed, using Flask dev server) ---")
        app.run(host='0.0.0.0', port=PORT, debug=False)