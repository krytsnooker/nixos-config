let state = { 
    answer: 0, 
    score: 0, 
    attempted: 0, 
    timeLeft: 0, // Changed to 0 initially
    interval: null, 
    currentInput: "" 
};

document.addEventListener('DOMContentLoaded', () => {
    // Fill Variable Dropdown
    const vc = document.getElementById('varCount');
    if (vc) {
        for(let i=2; i<=6; i++) vc.add(new Option(i, i));
        vc.value = 2;
    }

    // Fill Font size
    const fs = document.getElementById('fontSize');
    if (fs) {
        for(let i=12; i<=72; i+=4) fs.add(new Option(i, i));
        fs.value = 48;
    }

    // Generate Number Grid (1-12)
    const grid = document.getElementById('number-grid');
    if (grid) {
        const layout = [[1,2,3],[4,5,6],[7,8,9],[10,11,12]];
        layout.forEach(rowNums => {
            const div = document.createElement('div');
            div.className = 'num-row';
            rowNums.forEach(n => {
                div.innerHTML += `<div class="num-item"><span>${n}</span><input type="checkbox" class="num-chk" value="${n}" checked></div>`;
            });
            grid.appendChild(div);
        });
    }
    fetchNames();
});

/**
 * START SESSION
 */
function startSession() {
    // 1. CRITICAL FIX: Unfocus the button so 'Enter' doesn't click it again
    if (document.activeElement) document.activeElement.blur();

    // 2. Clear any existing timer
    if(state.interval) clearInterval(state.interval);
    
    // 3. Reset State
    state = { 
        answer: 0, 
        score: 0, 
        attempted: 0, 
        timeLeft: 60, 
        interval: null, 
        currentInput: "" 
    };
    
    document.getElementById('results').innerHTML = '';
    document.getElementById('input-container').style.display = 'block';
    document.getElementById('timer').textContent = '60s';
    
    clearInput();
    generate();
    
    state.interval = setInterval(() => {
        state.timeLeft--;
        document.getElementById('timer').textContent = state.timeLeft + 's';
        if(state.timeLeft <= 0) finish();
    }, 1000);
}

/**
 * MATH LOGIC
 */
function generate() {
    const mode = document.getElementById('mathMode').value;
    const allowed = Array.from(document.querySelectorAll('.num-chk:checked')).map(c => parseInt(c.value));
    const varCount = parseInt(document.getElementById('varCount').value);
    const area = document.getElementById('display-area');

    if (allowed.length === 0) {
        alert("Select numbers in sidebar!");
        if(state.interval) clearInterval(state.interval);
        return;
    }

    area.style.fontSize = document.getElementById('fontSize').value + 'px';
    area.style.color = "#eee"; // Reset color

    if (mode === 'division') {
        const b = allowed[Math.floor(Math.random() * allowed.length)];
        const ans = allowed[Math.floor(Math.random() * allowed.length)];
        state.answer = ans;
        area.innerHTML = `${b * ans} ÷ ${b} = `;
    } 
    else if (mode === 'multiplication') {
        const baseNumber = allowed[Math.floor(Math.random() * allowed.length)];
        const multiplier = Math.floor(Math.random() * 12) + 1; 
        state.answer = baseNumber * multiplier;
        area.innerHTML = `${baseNumber} × ${multiplier} = `;
    } 
    else {
        let vars = [];
        for(let i=0; i<varCount; i++) vars.push(allowed[Math.floor(Math.random() * allowed.length)]);
        const op = {addition:'+', subtraction:'-'}[mode];
        
        area.innerHTML = vars.join(` ${op} `) + " = ";
        state.answer = vars.reduce((acc, curr) => {
            if(mode === 'addition') return acc + curr;
            if(mode === 'subtraction') return acc - curr;
        });
    }
}

/**
 * INPUT HANDLING
 */
function numInput(n) {
    state.currentInput += n;
    document.getElementById('math-display-val').textContent = state.currentInput;
}

function clearInput() {
    state.currentInput = "";
    document.getElementById('math-display-val').textContent = "?";
}

function submitInput() {
    if (state.currentInput === "" || state.timeLeft <= 0) return;

    const val = parseInt(state.currentInput);
    state.attempted++;
    
    const area = document.getElementById('display-area');
    
    // Visual Feedback
    if(val === state.answer) {
        state.score++;
        area.style.color = "#888"; // Grayscale for correct
    } else {
        area.style.color = "red"; // Red for incorrect
    }

    // Delay slightly so user sees the color change
    setTimeout(() => {
        if (state.timeLeft > 0) {
            clearInput();
            generate();
        }
    }, 200);
}

/**
 * KEYBOARD SUPPORT
 */
window.addEventListener('keydown', (e) => {
    // Only capture keys if session is active
    if (state.timeLeft <= 0) return;

    // Ignore if typing in User Name box
    if (document.activeElement.id === 'userName') return;

    if (e.key >= '0' && e.key <= '9') {
        numInput(e.key);
    } 
    else if (e.key === 'Enter') {
        e.preventDefault(); // CRITICAL: Stop 'Enter' from clicking the Start button
        submitInput();
    } 
    else if (e.key === 'Backspace') {
        e.preventDefault();
        state.currentInput = state.currentInput.slice(0, -1);
        document.getElementById('math-display-val').textContent = state.currentInput || "?";
    }
    else if (e.key === 'Escape') {
        clearInput();
    }
});

/**
 * END SESSION & STATS
 */
async function finish() {
    if (state.interval) clearInterval(state.interval);
    state.interval = null;
    state.timeLeft = 0;

    document.getElementById('input-container').style.display = 'none';
    const acc = state.attempted > 0 ? (state.score / state.attempted).toFixed(2) : 0;
    
    document.getElementById('results').innerHTML = `
        <div style="background:#252526; padding:20px; border-radius:8px; border-left:4px solid #007acc">
            <h2>Time Up!</h2>
            <p><strong>Score:</strong> ${state.score} correct</p>
            <p><strong>Accuracy:</strong> ${Math.round(acc*100)}%</p>
        </div>
    `;
    
    await fetch('/api/log', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
            name: document.getElementById('userName').value || "Guest",
            mode: document.getElementById('mathMode').value,
            score: state.score, 
            accuracy: acc, 
            date: new Date().toISOString()
        })
    });
    fetchNames();
}

function toggleAll(v) { 
    document.querySelectorAll('.num-chk').forEach(c => c.checked = v); 
}

async function fetchNames() {
    try {
        const res = await fetch('/api/names');
        const names = await res.json();
        const dl = document.getElementById('nameOptions');
        if (dl) {
            dl.innerHTML = '';
            names.forEach(n => dl.add(new Option(n, n)));
        }
    } catch(e) {}
}

async function showStats() {
    document.getElementById('statsModal').style.display = 'block';
    try {
        const res = await fetch('/api/logs');
        const logs = await res.json();
        const tbody = document.querySelector('#statsTable tbody');
        tbody.innerHTML = '';
        logs.reverse().forEach(l => {
            tbody.innerHTML += `
                <tr>
                    <td>${new Date(l.date).toLocaleDateString()}</td>
                    <td>${l.name}</td>
                    <td>${l.mode}</td>
                    <td>${l.score}</td>
                    <td>${Math.round(l.accuracy*100)}%</td>
                </tr>`;
        });
    } catch(e) {}
}

function closeStats() { 
    document.getElementById('statsModal').style.display = 'none'; 
}

// Close modal when clicking outside
window.onclick = function(event) {
    const modal = document.getElementById('statsModal');
    if (event.target == modal) closeStats();
}