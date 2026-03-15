'use strict';

// ===== State =====
let ws = null;
let myName = '';
let myID = '';
let roomCode = '';
let muted = false;

// ===== WebRTC State =====
let pc = null;
let localStream = null;

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
  ws.send(JSON.stringify({ type, payload }));
}

// ===== Message Router =====

function handleMessage(msg) {
  const p = msg.payload || {};
  switch (msg.type) {
    case 'room-created':
      roomCode = p.code;
      myID = p.peerId;
      showScreen('screen-room');
      setRoomCode(p.code);
      break;

    case 'room-joined':
      roomCode = p.code;
      myID = p.peerId;
      showScreen('screen-room');
      setRoomCode(p.code);
      if (Array.isArray(p.peers)) {
        p.peers.forEach((peer) => addPeer(peer.id, peer.name, peer.muted));
      }
      break;

    case 'peer-joined':
      addPeer(p.id, p.name, false);
      break;

    case 'peer-left':
      removePeer(p.id);
      break;

    case 'peer-muted':
      updatePeerMute(p.id, p.muted);
      break;

    case 'offer':
      handleOffer(p);
      break;

    case 'ice-candidate':
      handleRemoteICECandidate(p);
      break;

    case 'error':
      showError(p.message || 'An unknown error occurred.');
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
  cleanupWebRTC();
  if (ws) {
    send('leave', {});
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
  send('mute', { muted });

  // Mute/unmute the local audio track sent over WebRTC.
  if (localStream) {
    localStream.getAudioTracks().forEach((track) => {
      track.enabled = !muted;
    });
  }

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

// ===== WebRTC =====

const rtcConfig = {
  iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
};

/**
 * Ensure we have a PeerConnection and local microphone stream.
 * Re-uses the existing pc/localStream if they are still alive.
 */
async function ensurePeerConnection() {
  // Acquire microphone if we haven't yet.
  if (!localStream) {
    try {
      localStream = await navigator.mediaDevices.getUserMedia({ audio: true });
    } catch (err) {
      showError('Microphone access denied. Voice will not work.');
      console.error('getUserMedia error:', err);
      return;
    }
  }

  if (!pc || pc.connectionState === 'closed') {
    pc = new RTCPeerConnection(rtcConfig);

    // Add local audio track so the SFU can receive our audio.
    localStream.getAudioTracks().forEach((track) => {
      pc.addTrack(track, localStream);
    });

    // Apply current mute state to the track.
    localStream.getAudioTracks().forEach((track) => {
      track.enabled = !muted;
    });

    // Send ICE candidates to the server as they are discovered (trickle ICE).
    pc.onicecandidate = (event) => {
      if (event.candidate) {
        send('ice-candidate', { candidate: event.candidate.candidate });
      }
    };

    // When the SFU forwards a remote track to us, play it.
    pc.ontrack = (event) => {
      const stream = event.streams[0] || new MediaStream([event.track]);
      playRemoteStream(stream);
    };

    pc.onconnectionstatechange = () => {
      console.log('WebRTC connection state:', pc.connectionState);
    };
  }
}

/**
 * Handle an SDP offer from the server.
 * Creates (or re-uses) a PeerConnection, sets the remote description,
 * creates an answer, and sends it back.
 */
async function handleOffer(payload) {
  try {
    await ensurePeerConnection();
    if (!pc) return; // getUserMedia was denied

    const offer = new RTCSessionDescription({ type: 'offer', sdp: payload.sdp });
    await pc.setRemoteDescription(offer);

    const answer = await pc.createAnswer();
    await pc.setLocalDescription(answer);

    send('answer', { sdp: answer.sdp });
  } catch (err) {
    console.error('handleOffer error:', err);
    showError('WebRTC negotiation failed.');
  }
}

/**
 * Handle an ICE candidate from the server.
 */
async function handleRemoteICECandidate(payload) {
  if (!pc) return;
  try {
    await pc.addIceCandidate(new RTCIceCandidate({ candidate: payload.candidate }));
  } catch (err) {
    console.error('addIceCandidate error:', err);
  }
}

/**
 * Play a remote audio stream by creating an <audio> element.
 * Each stream gets its own element (keyed by stream id to avoid duplicates).
 */
function playRemoteStream(stream) {
  // Avoid duplicate audio elements for the same stream.
  const containerId = 'remote-audio';
  let container = document.getElementById(containerId);
  if (!container) {
    container = document.createElement('div');
    container.id = containerId;
    container.style.display = 'none';
    document.body.appendChild(container);
  }

  const existingEl = container.querySelector(
    `[data-stream-id="${CSS.escape(stream.id)}"]`
  );
  if (existingEl) return;

  const audio = document.createElement('audio');
  audio.autoplay = true;
  audio.dataset.streamId = stream.id;
  audio.srcObject = stream;
  container.appendChild(audio);
}

/**
 * Clean up WebRTC resources (PeerConnection, local stream, remote audio).
 */
function cleanupWebRTC() {
  if (pc) {
    pc.close();
    pc = null;
  }
  if (localStream) {
    localStream.getTracks().forEach((track) => track.stop());
    localStream = null;
  }
  // Remove all remote audio elements.
  const container = document.getElementById('remote-audio');
  if (container) {
    container.remove();
  }
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
