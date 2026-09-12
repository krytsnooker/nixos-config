const letterRowsConfig = [
    ['A', 'S', 'D', 'F'], ['J', 'K', 'L', 'G', 'H'],
    ['T', 'U', 'Y', 'I', 'R'], ['E', 'O', 'P', 'W', 'Q'],
    ['V', 'N', 'C', 'B'], ['X', 'M', 'Z']
];

let state = {
    mode: "words", fullText: "", currentIndex: 0, isStarted: false,
    startTime: null, timerInterval: null, firstTimeCorrect: 0,
    errors: new Set(), timeLeft: 60
};

document.addEventListener('DOMContentLoaded', () => {
    initUI();
    fetchNames();
});

function initUI() {
    ['wordsPerLine', 'totalLines', 'visibleLines'].forEach(id => {
        const el = document.getElementById(id);
        if (el) {
            for (let i = 1; i <= 12; i++) el.add(new Option(i, i));
            el.value = 5;
        }
    });
    const fs = document.getElementById('fontSize');
    if (fs) {
        for (let i = 12; i <= 48; i += 2) fs.add(new Option(i, i));
        fs.value = 16;
    }
    const grid = document.getElementById('letter-grid');
    if (grid) {
        letterRowsConfig.forEach(rowChars => {
            const rowDiv = document.createElement('div');
            rowDiv.className = 'letter-row';
            rowChars.forEach(char => {
                const item = document.createElement('div');
                item.className = 'letter-item';
                item.innerHTML = `<span>${char}</span><input type="checkbox" class="letter-chk" value="${char.toLowerCase()}" checked>`;
                rowDiv.appendChild(item);
            });
            grid.appendChild(rowDiv);
        });
    }
}

function toggleModeUI() {
    const mode = document.getElementById('practiceMode').value;
    document.getElementById('wordSettings').style.display = (mode === 'chars') ? 'none' : 'block';
}

async function fetchNames() {
    try {
        const res = await fetch('/api/names');
        const names = await res.json();
        const dl = document.getElementById('nameOptions');
        if (dl) {
            dl.innerHTML = '';
            names.forEach(n => {
                const opt = document.createElement('option');
                opt.value = n;
                dl.appendChild(opt);
            });
        }
    } catch (e) {}
}

/** STATS MODAL LOGIC **/
async function showStats() {
    const modal = document.getElementById('statsModal');
    modal.style.display = 'block';
    try {
        const res = await fetch('/api/logs');
        const logs = await res.json();
        logs.reverse(); // Newest first

        const wordBody = document.querySelector('#wordStatsTable tbody');
        const charBody = document.querySelector('#charStatsTable tbody');
        wordBody.innerHTML = ''; charBody.innerHTML = '';

        logs.forEach(log => {
            const date = new Date(log.date).toLocaleDateString();
            const row = `<tr><td>${date}</td><td>${log.name}</td><td>${Math.round(log.accuracy * 100)}%</td><td>${log.speedCPS}</td></tr>`;
            if (log.mode === 'chars') charBody.innerHTML += row;
            else wordBody.innerHTML += row;
        });
    } catch (e) { console.error(e); }
}

function closeStats() {
    document.getElementById('statsModal').style.display = 'none';
}

// Close modal when clicking outside of it
window.onclick = function(event) {
    const modal = document.getElementById('statsModal');
    if (event.target == modal) closeStats();
}

/** TYPING LOGIC **/
async function startSession() {
    const mode = document.getElementById('practiceMode').value;
    const allowed = Array.from(document.querySelectorAll('.letter-chk:checked')).map(c => c.value);
    if (allowed.length === 0) return alert("Select letters!");

    const fSize = parseInt(document.getElementById('fontSize').value);
    let generatedText = "";
    
    if (mode === "words") {
        const res = await fetch(`/api/words?letters=${allowed.join('')}`);
        const wordPool = await res.json();
        if (wordPool.length === 0) return alert("No words match.");
        const wpl = parseInt(document.getElementById('wordsPerLine').value);
        const totalL = parseInt(document.getElementById('totalLines').value);
        let lines = [];
        for(let i=0; i<totalL; i++) {
            let line = [];
            for(let j=0; j<wpl; j++) line.push(wordPool[Math.floor(Math.random()*wordPool.length)]);
            lines.push(line.join(' '));
        }
        generatedText = lines.join('\n');
    } else {
        let chars = [];
        for(let i=0; i<800; i++) {
            chars.push(allowed[Math.floor(Math.random() * allowed.length)]);
            if (i > 0 && i % 10 === 0) chars.push('\n');
            else if (i > 0 && i % 2 === 0) chars.push(' ');
        }
        generatedText = chars.join('').trim();
    }

    state = {
        mode, userName: document.getElementById('userName').value || "Guest",
        fullText: generatedText, currentIndex: 0, isStarted: false,
        startTime: null, timerInterval: null, firstTimeCorrect: 0,
        errors: new Set(), timeLeft: 60
    };

    const area = document.getElementById('display-area');
    const visL = (mode === 'words') ? parseInt(document.getElementById('visibleLines').value) : 5;
    area.innerHTML = ''; area.style.fontSize = fSize + 'px';
    area.style.height = (fSize * 2.5 * visL) + 'px';
    document.getElementById('timer').textContent = (mode === 'words') ? '0.0s' : '60s';
    document.getElementById('results').innerHTML = '';

    state.fullText.split('').forEach((char, i) => {
        const span = document.createElement('span');
        span.className = 'char';
        span.id = `char-${i}`;
        span.textContent = char === '\n' ? '↵\n' : char;
        area.appendChild(span);
    });
    updateCursor();
}

window.addEventListener('keydown', (e) => {
    if(!state.fullText || state.currentIndex >= state.fullText.length) return;
    if(e.key.length !== 1 && e.key !== 'Enter') return;
    // Don't hijack keystrokes meant for the Name field (e.g. editing it mid-session)
    if (document.activeElement.id === 'userName') return;
    e.preventDefault();

    if(!state.isStarted) {
        state.isStarted = true;
        state.startTime = Date.now();
        state.timerInterval = setInterval(() => {
            const elapsed = (Date.now() - state.startTime)/1000;
            if (state.mode === 'words') {
                document.getElementById('timer').textContent = elapsed.toFixed(1) + 's';
            } else {
                state.timeLeft = 60 - Math.floor(elapsed);
                document.getElementById('timer').textContent = state.timeLeft + 's';
                if (state.timeLeft <= 0) finish();
            }
        }, 100);
    }

    const expected = state.fullText[state.currentIndex];
    const input = e.key === 'Enter' ? '\n' : e.key;
    const el = document.getElementById(`char-${state.currentIndex}`);

    if(input === expected) {
        if(!state.errors.has(state.currentIndex)) state.firstTimeCorrect++;
        el.className = 'char correct';
        state.currentIndex++;
        if(state.currentIndex === state.fullText.length && state.mode === 'words') finish();
        else updateCursor();
    } else {
        el.className = 'char incorrect';
        state.errors.add(state.currentIndex);
    }
});

function updateCursor() {
    document.querySelectorAll('.cursor').forEach(c => c.remove());
    const curEl = document.getElementById(`char-${state.currentIndex}`);
    if(curEl) {
        const cursor = document.createElement('div');
        cursor.className = 'cursor';
        curEl.appendChild(cursor);
        curEl.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
}

async function finish() {
    if (state.timerInterval) clearInterval(state.timerInterval);
    state.timerInterval = null;
    const duration = (Date.now() - state.startTime) / 1000;
    const totalAttempted = state.currentIndex;
    const acc = totalAttempted > 0 ? (state.firstTimeCorrect / totalAttempted).toFixed(2) : 0;
    const speed = duration > 0 ? (totalAttempted / duration).toFixed(2) : 0;

    document.getElementById('results').innerHTML = `<h3>Complete</h3><p>Accuracy: ${acc} | Speed: ${speed}</p>`;

    await fetch('/api/log', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
            id: Date.now(), name: state.userName, mode: state.mode,
            accuracy: acc, speedCPS: speed, date: new Date().toISOString()
        })
    });
    fetchNames();
}

function toggleAllLetters(checked) {
    document.querySelectorAll('.letter-chk').forEach(c => c.checked = checked);
}