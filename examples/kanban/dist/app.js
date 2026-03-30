// FILE: examples/kanban/dist/app.js
// PURPOSE: Run the filesystem-backed Luminka kanban board and sync it to kanban.json.
// OWNS: Board loading, rendering, title editing, persistence, and file-change reloads.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_luminka_phase3_surface_examples_2026-03-30.md

import { createLuminkaClient } from "./luminka.js";

const client = createLuminkaClient();
const boardPath = "kanban.json";
const saveDebounceMs = 200;

const connectBtn = document.getElementById("connect-btn");
const reloadBtn = document.getElementById("reload-btn");
const statusEl = document.getElementById("status");
const fileStateEl = document.getElementById("file-state");
const saveStateEl = document.getElementById("save-state");
const boardEl = document.getElementById("board");
const eventsEl = document.getElementById("events");

let board = createDefaultBoard();
let watching = false;
let saveTimer = null;

client.onFileChanged((path) => {
  if (path !== boardPath) {
    return;
  }
  appendEvent(`Detected change in ${path}`);
  void loadBoard({ fromWatch: true });
});

connectBtn.addEventListener("click", () => void connect());
reloadBtn.addEventListener("click", () => void loadBoard({ manual: true }));

void connect();

async function connect() {
  setStatus("Connecting...");
  try {
    await client.connect();
    setStatus("Connected.");
    await loadBoard({ manual: false });
  } catch (error) {
    setStatus(`Connection failed: ${messageOf(error)}`);
  }
}

async function loadBoard({ manual = false, fromWatch = false } = {}) {
  try {
    const exists = await client.exists(boardPath);
    if (exists) {
      const raw = await client.read(boardPath);
      board = parseBoard(raw);
      fileStateEl.textContent = `Loaded ${boardPath}`;
      saveStateEl.textContent = "In sync";
      saveStateEl.dataset.state = "synced";
    } else {
      board = createDefaultBoard();
      fileStateEl.textContent = `Using default board until first save to ${boardPath}`;
      saveStateEl.textContent = "In memory";
      saveStateEl.dataset.state = "memory";
    }

    renderBoard();

    if (!watching) {
      await client.watch(boardPath);
      watching = true;
      appendEvent(`Watching ${boardPath}`);
    }

    if (manual) {
      appendEvent(`Reloaded board from ${boardPath}`);
    } else if (fromWatch) {
      appendEvent(`Reloaded board after change notification`);
    }
  } catch (error) {
    setStatus(`Board load failed: ${messageOf(error)}`);
  }
}

function renderBoard() {
  boardEl.innerHTML = "";
  for (const column of board.columns) {
    const columnEl = document.createElement("section");
    columnEl.className = "column";
    columnEl.innerHTML = `
      <div class="column-head">
        <h3>${escapeHtml(column.title)}</h3>
        <span class="card-count">${column.cards.length} cards</span>
      </div>
    `;

    const cardsEl = document.createElement("div");
    cardsEl.className = "cards";

    for (const card of column.cards) {
      const cardEl = document.createElement("div");
      cardEl.className = "card";
      cardEl.innerHTML = `
        <label class="card-label" for="card-${card.id}">Card title</label>
        <input id="card-${card.id}" value="${escapeAttribute(card.title)}" />
      `;
      const input = cardEl.querySelector("input");
      input.addEventListener("input", () => {
        updateCardTitle(column.id, card.id, input.value);
        scheduleSave();
      });
      cardsEl.appendChild(cardEl);
    }

    columnEl.appendChild(cardsEl);
    boardEl.appendChild(columnEl);
  }
}

function updateCardTitle(columnId, cardId, title) {
  board = {
    ...board,
    columns: board.columns.map((column) => {
      if (column.id !== columnId) {
        return column;
      }
      return {
        ...column,
        cards: column.cards.map((card) => (card.id === cardId ? { ...card, title } : card)),
      };
    }),
  };
  saveStateEl.textContent = "Unsaved changes";
  saveStateEl.dataset.state = "dirty";
}

async function persistBoard() {
  try {
    await client.write(boardPath, JSON.stringify(board, null, 2));
    saveStateEl.textContent = "Saved to kanban.json";
    saveStateEl.dataset.state = "saved";
    appendEvent(`Saved ${boardPath}`);
  } catch (error) {
    setStatus(`Save failed: ${messageOf(error)}`);
  }
}

function scheduleSave() {
  if (saveTimer !== null) {
    clearTimeout(saveTimer);
  }
  saveTimer = window.setTimeout(() => {
    saveTimer = null;
    void persistBoard();
  }, saveDebounceMs);
}

function createDefaultBoard() {
  return {
    version: "1.0.0",
    app_name: "Luminka Kanban",
    columns: [
      {
        id: "todo",
        title: "To Do",
        cards: [
          { id: "todo-1", title: "Open the board in Luminka" },
          { id: "todo-2", title: "Edit a card title" },
        ],
      },
      {
        id: "doing",
        title: "Doing",
        cards: [{ id: "doing-1", title: "Watch kanban.json for reloads" }],
      },
      {
        id: "done",
        title: "Done",
        cards: [{ id: "done-1", title: "Use the SDK instead of raw WS" }],
      },
    ],
  };
}

function parseBoard(raw) {
  const parsed = JSON.parse(raw);
  if (!parsed || !Array.isArray(parsed.columns)) {
    throw new Error("kanban.json is missing the columns array");
  }
  return parsed;
}

function appendEvent(text) {
  const item = document.createElement("li");
  item.textContent = `${new Date().toLocaleTimeString()} — ${text}`;
  eventsEl.prepend(item);
}

function setStatus(text) {
  statusEl.textContent = text;
}

function messageOf(error) {
  return error instanceof Error ? error.message : String(error);
}

function escapeHtml(value) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function escapeAttribute(value) {
  return escapeHtml(value);
}
