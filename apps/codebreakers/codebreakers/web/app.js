const NUM_COLORS = 6;
const CODE_LENGTH = 4;
const LOBBY_POLL_MS = 3000;

const state = {
  gameId: null,
  playerId: null,
  codePegs: new Array(CODE_LENGTH).fill(0),
  guessPegs: new Array(CODE_LENGTH).fill(0),
  guessTurnKey: null,
  socket: null,
  lobbyTimer: null,
};

const el = (id) => document.getElementById(id);

function buildPegRow(container, pegs, onPick, lockedMask) {
  container.innerHTML = "";
  pegs.forEach((val, i) => {
    const slot = document.createElement("div");
    slot.className = "peg-slot";
    const locked = lockedMask && lockedMask[i];
    if (val > 0) slot.dataset.peg = val;
    slot.textContent = val > 0 ? "" : "?";
    if (locked) {
      slot.classList.add("locked");
    } else {
      slot.addEventListener("click", () => {
        const next = (pegs[i] % NUM_COLORS) + 1;
        pegs[i] = next;
        onPick();
      });
    }
    container.appendChild(slot);
  });
}

function renderCodeSetup() {
  buildPegRow(el("code-pegs"), state.codePegs, renderCodeSetup);
}

function knownPegsFromHistory(history) {
  const known = new Array(CODE_LENGTH).fill(0);
  history.forEach((g) => {
    (g.feedback.exactPositions || []).forEach((idx) => {
      known[idx] = g.pegs[idx];
    });
  });
  return known;
}

function disabledColorsByPosition(history) {
  const disabled = Array.from({ length: CODE_LENGTH }, () => new Set());
  history.forEach((g) => {
    const exact = new Set(g.feedback.exactPositions || []);
    g.pegs.forEach((p, i) => {
      if (!exact.has(i)) disabled[i].add(p);
    });
  });
  return disabled;
}

function renderGuessSetup(view) {
  const known = knownPegsFromHistory(view.history);
  const lockedMask = known.map((v) => v > 0);
  const disabled = disabledColorsByPosition(view.history);

  const turnKey = view.history.length;
  if (state.guessTurnKey !== turnKey) {
    state.guessTurnKey = turnKey;
    state.guessPegs = known.map((v, i) => (lockedMask[i] ? v : 0));
  }

  const container = el("guess-positions");
  container.innerHTML = "";

  for (let i = 0; i < CODE_LENGTH; i++) {
    const posWrap = document.createElement("div");
    posWrap.className = "guess-position";

    if (lockedMask[i]) {
      const slot = document.createElement("div");
      slot.className = "peg-slot locked";
      slot.dataset.peg = known[i];
      posWrap.appendChild(slot);
    } else {
      const palette = document.createElement("div");
      palette.className = "palette";
      for (let c = 1; c <= NUM_COLORS; c++) {
        const swatch = document.createElement("div");
        swatch.className = "palette-swatch";
        swatch.dataset.peg = c;
        const isDisabled = disabled[i].has(c);
        if (isDisabled) {
          swatch.classList.add("disabled");
        } else {
          if (state.guessPegs[i] === c) swatch.classList.add("selected");
          swatch.addEventListener("click", () => {
            state.guessPegs[i] = c;
            renderGuessSetup(view);
          });
        }
        palette.appendChild(swatch);
      }
      posWrap.appendChild(palette);
    }

    container.appendChild(posWrap);
  }
}

function renderBoard(view) {
  const board = el("board");
  board.innerHTML = "";
  view.history.forEach((g) => {
    const row = document.createElement("div");
    row.className = "guess-row";

    const name = document.createElement("span");
    name.className = "guess-player";
    name.textContent = playerName(view, g.playerId);
    row.appendChild(name);

    const pegRow = document.createElement("div");
    pegRow.className = "peg-row";
    g.pegs.forEach((p) => {
      const slot = document.createElement("div");
      slot.className = "peg-slot";
      slot.dataset.peg = p;
      pegRow.appendChild(slot);
    });
    row.appendChild(pegRow);

    const fb = document.createElement("div");
    fb.className = "feedback";
    for (let i = 0; i < g.feedback.black; i++) {
      const dot = document.createElement("div");
      dot.className = "fb-dot black";
      fb.appendChild(dot);
    }
    for (let i = 0; i < g.feedback.white; i++) {
      const dot = document.createElement("div");
      dot.className = "fb-dot white";
      fb.appendChild(dot);
    }
    row.appendChild(fb);

    board.appendChild(row);
  });
}

function playerName(view, playerId) {
  if (playerId === view.makerId) return view.makerName || "Codemaker";
  const b = view.breakers.find((p) => p.id === playerId);
  return b ? b.name : "???";
}

function currentTurnPlayerId(view) {
  if (view.breakers.length === 0) return null;
  return view.breakers[view.turnIndex % view.breakers.length].id;
}

function renderPlayers(view) {
  const names = view.breakers.map((p) => p.name).join(", ") || "none yet";
  el("players").textContent = `Codemaker: ${view.makerName || "???"} — Codebreakers: ${names}`;
}

function renderStatus(view) {
  const labels = {
    waiting_for_code: "Waiting for codemaker to set the secret code...",
    in_progress: "Game in progress",
    breakers_won: "Codebreakers won!",
    maker_won: "Codemaker won!",
  };
  el("status-label").textContent = labels[view.status] || view.status;

  if ((view.status === "breakers_won" || view.status === "maker_won") && view.code) {
    el("status-label").textContent += ` (code was ${view.code.join("-")})`;
  }
}

function render(view) {
  el("game-id-label").textContent = state.gameId;
  renderStatus(view);
  renderPlayers(view);

  const amMaker = view.makerId === state.playerId;
  el("role-badge").textContent = amMaker ? "Codemaker" : "Codebreaker";
  el("role-badge").classList.remove("hidden");

  const waitingForCode = view.status === "waiting_for_code";
  el("code-setup").classList.toggle("hidden", !(amMaker && waitingForCode));
  el("code-reveal").classList.toggle("hidden", !(amMaker && !waitingForCode && view.code));
  if (amMaker && !waitingForCode && view.code) {
    buildPegRow(el("code-reveal-pegs"), view.code, () => {}, [true, true, true, true]);
  }

  el("board-wrap").classList.toggle("hidden", waitingForCode);

  renderBoard(view);

  const myTurn = !amMaker && view.status === "in_progress" && currentTurnPlayerId(view) === state.playerId;
  el("guess-setup").classList.toggle("hidden", !myTurn);
  if (myTurn) renderGuessSetup(view);
}

function connectSocket() {
  const proto = location.protocol === "https:" ? "wss" : "ws";
  const socket = new WebSocket(`${proto}://${location.host}/ws/${state.gameId}?playerId=${state.playerId}`);
  socket.onmessage = (ev) => {
    const data = JSON.parse(ev.data);
    if (data.terminated) {
      alert("This game has been terminated.");
      leaveGame({ alreadyGone: true });
      return;
    }
    render(data);
  };
  state.socket = socket;
}

function showGame() {
  if (state.lobbyTimer) {
    clearInterval(state.lobbyTimer);
    state.lobbyTimer = null;
  }
  el("lobby").classList.add("hidden");
  el("game").classList.remove("hidden");
  state.guessPegs = new Array(CODE_LENGTH).fill(0);
  state.guessTurnKey = null;
  renderCodeSetup();
  connectSocket();
}

async function leaveGame(opts = {}) {
  if (!opts.alreadyGone && state.gameId && state.playerId) {
    try {
      await postJSON(`/api/games/${state.gameId}/leave`, { playerId: state.playerId });
    } catch (e) {
      // game may already be gone (e.g. terminated concurrently) — proceed to local cleanup regardless
    }
  }

  if (state.socket) {
    state.socket.close();
    state.socket = null;
  }
  state.gameId = null;
  state.playerId = null;
  state.guessPegs = new Array(CODE_LENGTH).fill(0);
  state.guessTurnKey = null;

  el("game").classList.add("hidden");
  el("lobby").classList.remove("hidden");
  el("role-badge").classList.add("hidden");

  refreshGameList();
  if (!state.lobbyTimer) {
    state.lobbyTimer = setInterval(refreshGameList, LOBBY_POLL_MS);
  }
}

async function postJSON(url, body) {
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || "request failed");
  return data;
}

async function terminateGame(gameId, makerName) {
  if (!confirm(`End ${makerName}'s game for everyone?`)) return;
  try {
    await fetch(`/api/games/${gameId}`, { method: "DELETE" });
    refreshGameList();
  } catch (e) {
    showLobbyError(e.message);
  }
}

function showLobbyError(message) {
  el("lobby-error").textContent = message;
  el("lobby-error").classList.remove("hidden");
}

async function refreshGameList() {
  try {
    const res = await fetch("/api/games");
    const games = await res.json();
    renderGameList(games || []);
  } catch (e) {
    // transient fetch failures are not worth surfacing during lobby polling
  }
}

function renderGameList(games) {
  const list = el("game-list");
  list.innerHTML = "";

  if (games.length === 0) {
    const empty = document.createElement("p");
    empty.className = "game-list-empty";
    empty.textContent = "No games in session. Create one!";
    list.appendChild(empty);
    return;
  }

  const statusLabels = {
    waiting_for_code: "waiting for code",
    in_progress: "in progress",
  };

  games.forEach((g) => {
    const row = document.createElement("div");
    row.className = "game-row";

    const info = document.createElement("div");
    info.className = "game-info";
    const title = document.createElement("span");
    title.textContent = `${g.makerName}'s game`;
    const sub = document.createElement("small");
    sub.textContent = `${statusLabels[g.status] || g.status} · ${g.numBreakers} player(s)`;
    info.appendChild(title);
    info.appendChild(sub);
    row.appendChild(info);

    const actions = document.createElement("div");
    actions.className = "game-row-actions";

    const joinBtn = document.createElement("button");
    joinBtn.textContent = "Join";
    joinBtn.addEventListener("click", () => joinGame(g.id));
    actions.appendChild(joinBtn);

    const endBtn = document.createElement("button");
    endBtn.className = "secondary-btn";
    endBtn.textContent = "End";
    endBtn.addEventListener("click", () => terminateGame(g.id, g.makerName));
    actions.appendChild(endBtn);

    row.appendChild(actions);

    list.appendChild(row);
  });
}

async function joinGame(gameId) {
  const name = el("name-input").value.trim() || "Codebreaker";
  try {
    const data = await postJSON(`/api/games/${gameId}/join`, { name });
    state.gameId = gameId;
    state.playerId = data.playerId;
    showGame();
  } catch (e) {
    showLobbyError(e.message);
  }
}

el("create-btn").addEventListener("click", async () => {
  const name = el("name-input").value.trim() || "Codemaker";
  try {
    const data = await postJSON("/api/games", { name });
    state.gameId = data.gameId;
    state.playerId = data.playerId;
    showGame();
  } catch (e) {
    showLobbyError(e.message);
  }
});

el("submit-code-btn").addEventListener("click", async () => {
  try {
    await postJSON(`/api/games/${state.gameId}/code`, {
      playerId: state.playerId,
      pegs: state.codePegs,
    });
  } catch (e) {
    alert(e.message);
  }
});

el("submit-guess-btn").addEventListener("click", async () => {
  try {
    await postJSON(`/api/games/${state.gameId}/guess`, {
      playerId: state.playerId,
      pegs: state.guessPegs,
    });
  } catch (e) {
    alert(e.message);
  }
});

el("leave-btn").addEventListener("click", leaveGame);

refreshGameList();
state.lobbyTimer = setInterval(refreshGameList, LOBBY_POLL_MS);
