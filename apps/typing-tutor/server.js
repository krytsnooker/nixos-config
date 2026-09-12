const express = require('express');
const fs = require('fs');
const path = require('path');

const app = express();
const PORT = process.env.TYPING_TUTOR_PORT || 3000;

const WORDS_FILE = path.join(__dirname, 'words.json'); // read-only, fine on a Nix store path
// Must be overridden to a writable path (e.g. systemd StateDirectory) when
// the code itself is on a read-only filesystem, like a Nix store path.
const LOGS_FILE = process.env.TYPING_TUTOR_LOGS_FILE || path.join(__dirname, 'logs.json');
const PUBLIC_DIR = path.join(__dirname, 'public');

app.use(express.json());
app.use(express.static(PUBLIC_DIR));

// API: Get words based on allowed characters
app.get('/api/words', (req, res) => {
    const allowed = (req.query.letters || "").toLowerCase();
    try {
        if (!fs.existsSync(WORDS_FILE)) return res.status(404).json([]);
        const words = JSON.parse(fs.readFileSync(WORDS_FILE, 'utf8'));
        const filtered = words.filter(w => w.split('').every(c => allowed.includes(c)));
        res.json(filtered);
    } catch (e) {
        res.status(500).json([]);
    }
});

// API: Get unique names for the datalist
app.get('/api/names', (req, res) => {
    try {
        if (!fs.existsSync(LOGS_FILE)) return res.json([]);
        const logs = JSON.parse(fs.readFileSync(LOGS_FILE, 'utf8') || '[]');
        const names = [...new Set(logs.map(l => l.name))].filter(n => n);
        res.json(names);
    } catch (e) {
        res.json([]);
    }
});

// API: Get ALL logs for the Stats Popup
app.get('/api/logs', (req, res) => {
    try {
        if (!fs.existsSync(LOGS_FILE)) return res.json([]);
        const data = fs.readFileSync(LOGS_FILE, 'utf8');
        res.json(JSON.parse(data || '[]'));
    } catch (e) {
        res.status(500).json([]);
    }
});

// API: Save session log
app.post('/api/log', (req, res) => {
    try {
        let logs = [];
        if (fs.existsSync(LOGS_FILE)) {
            const data = fs.readFileSync(LOGS_FILE, 'utf8');
            logs = JSON.parse(data || '[]');
        }
        logs.push(req.body);
        fs.writeFileSync(LOGS_FILE, JSON.stringify(logs, null, 2));
        res.sendStatus(200);
    } catch (e) {
        res.status(500).send("Error saving log");
    }
});

const server = app.listen(PORT, '0.0.0.0', () => {
    console.log(`Server: http://localhost:${PORT}\nPress Ctrl+C to stop.`);
});

const shutdown = () => {
    server.close(() => {
        console.log('\nPort 3000 released.');
        process.exit(0);
    });
};
process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);