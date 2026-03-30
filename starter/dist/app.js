// FILE: starter/dist/app.js
// PURPOSE: Demonstrate the Luminka SDK with connect, app info, filesystem, and watch flows.
// OWNS: Starter UI behavior, local note editing, directory listing, and change notifications.
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_luminka_phase3_surface_examples_2026-03-30.md

import { createLuminkaClient } from "./luminka.js";

const client = createLuminkaClient();
const notePath = "starter-note.txt";
let watching = false;

const connectBtn = document.getElementById("connect-btn");
const infoBtn = document.getElementById("info-btn");
const writeBtn = document.getElementById("write-btn");
const readBtn = document.getElementById("read-btn");
const watchBtn = document.getElementById("watch-btn");
const unwatchBtn = document.getElementById("unwatch-btn");
const listBtn = document.getElementById("list-btn");
const statusEl = document.getElementById("status");
const appInfoEl = document.getElementById("app-info");
const noteInput = document.getElementById("note-input");
const noteOutput = document.getElementById("note-output");
const fileList = document.getElementById("file-list");
const events = document.getElementById("events");

client.onFileChanged((path) => {
  appendEvent(`Changed: ${path}`);
  if (path === notePath) {
    void refreshNote();
  }
});

connectBtn.addEventListener("click", () => void connect());
infoBtn.addEventListener("click", () => void refreshAppInfo());
writeBtn.addEventListener("click", () => void writeNote());
readBtn.addEventListener("click", () => void refreshNote());
watchBtn.addEventListener("click", () => void startWatch());
unwatchBtn.addEventListener("click", () => void stopWatch());
listBtn.addEventListener("click", () => void refreshListing());

async function connect() {
  setStatus("Connecting...");
  await client.connect();
  setStatus("Connected.");
  await refreshAppInfo();
  await refreshListing();
}

async function refreshAppInfo() {
  const info = await client.appInfo();
  appInfoEl.textContent = JSON.stringify(info, null, 2);
  setStatus(`Connected to ${info.name} (${info.mode}).`);
}

async function writeNote() {
  await client.write(notePath, noteInput.value);
  appendEvent(`Wrote ${notePath}`);
  noteOutput.textContent = noteInput.value;
}

async function refreshNote() {
  const text = await client.read(notePath);
  noteOutput.textContent = text;
  noteInput.value = text;
}

async function refreshListing() {
  const files = await client.list();
  fileList.textContent = files.length ? files.join("\n") : "(empty)";
}

async function startWatch() {
  if (watching) {
    appendEvent(`Already watching ${notePath}`);
    return;
  }
  await client.watch(notePath);
  watching = true;
  appendEvent(`Watching ${notePath}`);
}

async function stopWatch() {
  if (!watching) {
    appendEvent(`Not watching ${notePath}`);
    return;
  }
  await client.unwatch(notePath);
  watching = false;
  appendEvent(`Stopped watching ${notePath}`);
}

function appendEvent(text) {
  const item = document.createElement("li");
  item.textContent = `${new Date().toLocaleTimeString()} — ${text}`;
  events.prepend(item);
}

function setStatus(text) {
  statusEl.textContent = text;
}
