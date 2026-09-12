# database.py
import sqlite3
from contextlib import closing
from config import DB_PATH


def get_db():
    return closing(sqlite3.connect(DB_PATH))


def init_db():
    """Initializes all tables and performance indexes.

    NOTE: this intentionally does NOT create `tracks` — that table is owned
    by music_scanner.py (it also fills it). If music.db is ever recreated
    from scratch without running the scanner first, app.py's queries against
    `tracks` will fail until the scanner runs.
    """
    with get_db() as conn:
        # 1. Main Metadata Cache (MusicBrainz/Wiki)
        conn.execute('''CREATE TABLE IF NOT EXISTS album_metadata
                        (artist TEXT, album TEXT, year TEXT, country TEXT, label TEXT, mbid TEXT,
                         artist_summary TEXT, album_summary TEXT, discogs_id TEXT,
                         PRIMARY KEY (artist, album))''')

        # 2. Entities (Unique People/Companies for 'Where Used')
        conn.execute('''CREATE TABLE IF NOT EXISTS entities
                        (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT UNIQUE)''')

        # 3. Link Tables (Discogs Relational Data)
        conn.execute('''CREATE TABLE IF NOT EXISTS album_credits
                        (artist TEXT, album TEXT, entity_name TEXT, role TEXT)''')
        conn.execute('''CREATE TABLE IF NOT EXISTS album_companies
                        (artist TEXT, album TEXT, entity_name TEXT, entity_type TEXT)''')

        # 4. Performance Indexes (Essential for 'Where Used' speed)
        conn.execute('CREATE INDEX IF NOT EXISTS idx_credits_entity ON album_credits (entity_name)')
        conn.execute('CREATE INDEX IF NOT EXISTS idx_companies_entity ON album_companies (entity_name)')
        conn.commit()


def query_db(query, args=(), one=False):
    """Standard helper to query the database."""
    with get_db() as conn:
        rv = conn.execute(query, args).fetchall()
    return (rv[0] if rv else None) if one else rv


def save_discogs_to_db(artist, album, data):
    """Saves detailed Discogs data into the relational tables."""
    with get_db() as conn:
        # Process Credits (extraartists)
        for person in data.get('extraartists', []):
            conn.execute("INSERT OR IGNORE INTO entities (name) VALUES (?)", (person['name'],))
            conn.execute("INSERT INTO album_credits VALUES (?, ?, ?, ?)",
                         (artist, album, person['name'], person['role']))

        # Process Companies
        for company in data.get('companies', []):
            conn.execute("INSERT OR IGNORE INTO entities (name) VALUES (?)", (company['name'],))
            conn.execute("INSERT INTO album_companies VALUES (?, ?, ?, ?)",
                         (artist, album, company['name'], company['entity_type_name']))

        # Link the Discogs ID back to the main metadata record
        conn.execute("UPDATE album_metadata SET discogs_id = ? WHERE artist = ? AND album = ?",
                     (data['id'], artist, album))
        conn.commit()


def get_where_used(entity_name):
    """Finds all albums associated with a specific person or company."""
    query = """
    SELECT artist, album, role as association_type FROM album_credits WHERE entity_name = ?
    UNION
    SELECT artist, album, entity_type as association_type FROM album_companies WHERE entity_name = ?
    ORDER BY artist, album
    """
    return query_db(query, [entity_name, entity_name])
