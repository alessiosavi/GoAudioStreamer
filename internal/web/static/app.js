'use strict';

// ===== State =====
let ws = null;
let myName = '';
let myID = '';
let roomCode = '';
let muted = false;

// ===== WebSocket =====

function connect(onOpen) {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(`${proto}//${location.host}/ws`);

  ws.addEventListener('open', () => {
    if (onOpen) onOpen();
  });

  ws.addEventListener('message', (evt) => {
    let msg;
    try {
      msg = JSON.parse(evt.data);
    } catch (_) {
      return;
    }
    handleMessage(msg);
  });

  ws.addEventListener('close', () => {
    ws = null;
    if (roomCode) {
      showError('Connection lost. Please rejoin.');
    }
  });

  ws.addEventListener('error', () => {
    showError('WebSocket error. Check your connection.');
  });
}

function send(type, payload) {
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    showError('Not connected to server.');
    return;
  }
  ws.send(JSON.stringify({ type, ...payload }));
}

// ===== Message Router =====

function handleMessage(msg) {
  switch (msg.type) {
    case 'room-created':
      roomCode = msg.code;
      myID = msg.peerId;
      showScreen('screen-room');
      setRoomCode(msg.code);
      break;

    case 'room-joined':
      roomCode = msg.code;
      myID = msg.peerId;
      showScreen('screen-room');
      setRoomCode(msg.code);
      if (Array.isArray(msg.peers)) {
        msg.peers.forEach((p) => addPeer(p.id, p.name, p.muted));
      }
      break;

    case 'peer-joined':
      addPeer(msg.peerId, msg.name, false);
      break;

    case 'peer-left':
      removePeer(msg.peerId);
      break;

    case 'peer-muted':
      updatePeerMute(msg.peerId, msg.muted);
      break;

    case 'error':
      showError(msg.message || 'An unknown error occurred.');
      break;

    default:
      break;
  }
}

// ===== Room Actions =====

function createRoom() {
  myName = document.getElementById('input-name').value.trim();
  if (!myName) {
    showError('Please enter your name.');
    return;
  }
  connect(() => send('create-room', { name: myName }));
}

function joinRoom() {
  myName = document.getElementById('input-name').value.trim();
  const code = document.getElementById('input-code').value.trim().toUpperCase();
  if (!myName) {
    showError('Please enter your name.');
    return;
  }
  if (!code) {
    showError('Please enter a room code.');
    return;
  }
  connect(() => send('join-room', { name: myName, code }));
}

function leaveRoom() {
  if (ws) {
    send('leave-room', {});
    ws.close();
    ws = null;
  }
  roomCode = '';
  myID = '';
  muted = false;
  clearPeerList();
  resetMuteButton();
  showScreen('screen-lobby');
}

function toggleMute() {
  muted = !muted;
  send('set-mute', { muted });

  // Also notify the local audio controller via REST
  fetch('/api/audio/mute', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ muted }),
  }).catch(() => {/* best-effort */});

  updatePeerMute(myID, muted);
  const btn = document.getElementById('btn-mute');
  btn.textContent = muted ? 'Unmute' : 'Mute';
  btn.classList.toggle('btn-muted', muted);
}

function copyCode() {
  if (!roomCode) return;
  navigator.clipboard.writeText(roomCode).catch(() => {/* ignore */});
}

function resetMuteButton() {
  const btn = document.getElementById('btn-mute');
  btn.textContent = 'Mute';
  btn.classList.remove('btn-muted');
}

// ===== Peer List DOM (safe — no innerHTML with user data) =====

function addPeer(id, name, isMuted) {
  const list = document.getElementById('peer-list');

  // Remove existing entry with same id (dedup)
  removePeer(id);

  const li = document.createElement('li');
  li.className = 'peer-card' + (isMuted ? ' muted' : '');
  li.dataset.peerId = id;

  const avatar = document.createElement('div');
  avatar.className = 'peer-avatar';
  // Use first character of name as avatar letter — safe via textContent
  avatar.textContent = name.charAt(0).toUpperCase() || '?';

  const nameEl = document.createElement('span');
  nameEl.className = 'peer-name';
  nameEl.textContent = name;

  const status = document.createElement('span');
  status.className = 'peer-status';
  status.textContent = isMuted ? 'muted' : 'speaking';

  li.appendChild(avatar);
  li.appendChild(nameEl);
  li.appendChild(status);
  list.appendChild(li);
}

function removePeer(id) {
  const list = document.getElementById('peer-list');
  const existing = list.querySelector(`[data-peer-id="${CSS.escape(id)}"]`);
  if (existing) existing.remove();
}

function updatePeerMute(id, isMuted) {
  const list = document.getElementById('peer-list');
  const card = list.querySelector(`[data-peer-id="${CSS.escape(id)}"]`);
  if (!card) return;

  card.classList.toggle('muted', isMuted);
  const status = card.querySelector('.peer-status');
  if (status) {
    status.textContent = isMuted ? 'muted' : 'speaking';
  }
}

function clearPeerList() {
  const list = document.getElementById('peer-list');
  while (list.firstChild) list.removeChild(list.firstChild);
}

// ===== Screen Management =====

function showScreen(id) {
  document.querySelectorAll('.screen').forEach((el) => {
    el.classList.toggle('active', el.id === id);
  });
}

function setRoomCode(code) {
  document.getElementById('display-code').textContent = code;
}

// ===== Error Overlay =====

function showError(msg) {
  const overlay = document.getElementById('error-overlay');
  const msgEl = document.getElementById('error-msg');
  msgEl.textContent = msg; // safe — textContent
  overlay.classList.remove('hidden');
}

function dismissError() {
  document.getElementById('error-overlay').classList.add('hidden');
}

// ===== Event Wiring =====

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('btn-create').addEventListener('click', createRoom);
  document.getElementById('btn-join').addEventListener('click', joinRoom);
  document.getElementById('btn-leave').addEventListener('click', leaveRoom);
  document.getElementById('btn-mute').addEventListener('click', toggleMute);
  document.getElementById('btn-copy').addEventListener('click', copyCode);
  document.getElementById('btn-dismiss').addEventListener('click', dismissError);

  // Allow pressing Enter in code input to trigger join
  document.getElementById('input-code').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') joinRoom();
  });

  // Allow pressing Enter in name input to create room
  document.getElementById('input-name').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') createRoom();
  });
});
