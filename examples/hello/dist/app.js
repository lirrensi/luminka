// FILE: examples/hello/dist/app.js
// PURPOSE: Demonstrate the Luminka SDK without filesystem access.
// OWNS: Hello example connection flow and app-info rendering.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_luminka_phase3_surface_examples_2026-03-30.md

import { createLuminkaClient } from "./luminka.js";

const client = createLuminkaClient();

const connectBtn = document.getElementById("connect-btn");
const refreshBtn = document.getElementById("refresh-btn");
const statusEl = document.getElementById("status");
const infoEl = document.getElementById("info");
const fsStateEl = document.getElementById("fs-state");

connectBtn.addEventListener("click", () => void connect());
refreshBtn.addEventListener("click", () => void refreshInfo());

void connect();

async function connect() {
  setStatus("Connecting...");
  try {
    await client.connect();
    setStatus("Connected.");
    await refreshInfo();
  } catch (error) {
    setStatus(`Connection failed: ${messageOf(error)}`);
  }
}

async function refreshInfo() {
  try {
    const info = await client.appInfo();
    infoEl.textContent = JSON.stringify(info, null, 2);
    fsStateEl.textContent = info.capabilities.fs ? "Filesystem capability: enabled" : "Filesystem capability: disabled";
    fsStateEl.dataset.state = info.capabilities.fs ? "enabled" : "disabled";
    setStatus(`Hello app ready: ${info.name}`);
  } catch (error) {
    setStatus(`App info failed: ${messageOf(error)}`);
  }
}

function setStatus(text) {
  statusEl.textContent = text;
}

function messageOf(error) {
  return error instanceof Error ? error.message : String(error);
}
