const express = require('express');
const fs = require('fs');
const path = require('path');
const app = express();
const PORT = process.env.MATH_TUTOR_PORT || 3001;

// Defaults next to the script for local/dev use, but must be overridden to a
// writable path (e.g. systemd StateDirectory) when the code itself is on a
// read-only filesystem, like a Nix store path.
const LOGS_FILE = process.env.MATH_TUTOR_LOGS_FILE || path.join(__dirname, 'logs.json');
app.use(express.json());
app.use(express.static(path.join(__dirname, 'public')));

if (!fs.existsSync(LOGS_FILE)) fs.writeFileSync(LOGS_FILE, JSON.stringify([]));

app.get('/api/names', (req, res) => {
    try {
        const logs = JSON.parse(fs.readFileSync(LOGS_FILE, 'utf8') || '[]');
        const names = [...new Set(logs.map(l => l.name))].filter(n => n);
        res.json(names);
    } catch (e) { res.json([]); }
});

app.get('/api/logs', (req, res) => {
    try {
        const data = fs.readFileSync(LOGS_FILE, 'utf8');
        res.json(JSON.parse(data || '[]'));
    } catch (e) { res.json([]); }
});

app.post('/api/log', (req, res) => {
    try {
        const logs = JSON.parse(fs.readFileSync(LOGS_FILE, 'utf8') || '[]');
        logs.push(req.body);
        fs.writeFileSync(LOGS_FILE, JSON.stringify(logs, null, 2));
        res.sendStatus(200);
    } catch (e) { res.status(500).send("Error"); }
});

const server = app.listen(PORT, '0.0.0.0', () => {
    console.log(`Math Game: http://localhost:${PORT}`);
});

process.on('SIGINT', () => {
    server.close(() => { process.exit(0); });
});