import os
import sqlite3
from contextlib import closing

from flask import Flask, render_template, request, redirect, url_for

app = Flask(__name__)

DB_PATH = os.environ.get('PASTEBIN_DB_PATH', 'pastes.db')
MAX_PASTES_SHOWN = 500


def get_db():
    return closing(sqlite3.connect(DB_PATH))


def init_db():
    with get_db() as conn:
        conn.execute('''CREATE TABLE IF NOT EXISTS pastes
                     (id INTEGER PRIMARY KEY AUTOINCREMENT, content TEXT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP)''')
        conn.commit()


@app.route('/', methods=['GET', 'POST'])
def index():
    if request.method == 'POST':
        content = request.form.get('content')
        if content:
            with get_db() as conn:
                conn.execute("INSERT INTO pastes (content) VALUES (?)", (content,))
                conn.commit()
        return redirect(url_for('index'))

    with get_db() as conn:
        pastes = conn.execute(
            "SELECT id, content, timestamp FROM pastes ORDER BY timestamp DESC LIMIT ?",
            (MAX_PASTES_SHOWN,),
        ).fetchall()
    return render_template('index.html', pastes=pastes)


if __name__ == '__main__':
    init_db()
    port = int(os.environ.get('PASTEBIN_PORT', 5001))
    try:
        from waitress import serve
        print(f"Starting LAN Pastebin (waitress) on 0.0.0.0:{port}")
        serve(app, host='0.0.0.0', port=port)
    except ImportError:
        print("waitress not installed, falling back to Flask's dev server (not recommended for always-on use)")
        app.run(host='0.0.0.0', port=port, debug=False)
