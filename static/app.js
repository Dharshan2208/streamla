let isHost = false;
let hostToken = sessionStorage.getItem("streamla_token") || "";
let ws = null;
let isProgrammaticChange = false; // prevents echo loops

const video = document.getElementById("player");
const setupCard = document.getElementById("setup-card");
const videoWrapper = document.getElementById("video-wrapper");
const movieStatus = document.getElementById("movie-status");
const movieInput = document.getElementById("movie-input");
const uploadBtn = document.getElementById("upload-btn");
const fileNameSpan = document.getElementById("file-name");
const joinSection = document.getElementById("join-section");
const joinBtn = document.getElementById("join-btn");
const roomBadge = document.getElementById("room-badge");
const roomIdSpan = document.getElementById("room-id");
const viewersBadge = document.getElementById("viewers-badge");
const viewerCountSpan = document.getElementById("viewer-count");
const roleTag = document.getElementById("role-tag");
const copyBtn = document.getElementById("copy-btn");

// 1. Initial State Check
async function init() {
  try {
    const res = await fetch("/api/info");
    const info = await res.json();
    roomIdSpan.textContent = info.roomId;

    if (info.hasMovie) {
      movieStatus.textContent = `Active Movie: ${info.filename}`;
      joinSection.classList.remove("hidden");
    } else {
      movieStatus.textContent = "No movie selected. Host must upload a movie.";
    }
  } catch (err) {
    movieStatus.textContent = "Unable to connect to server.";
  }
}

// 2. File Selection & Upload
movieInput.addEventListener("change", () => {
  if (movieInput.files.length > 0) {
    fileNameSpan.textContent = movieInput.files[0].name;
    uploadBtn.disabled = false;
  }
});

uploadBtn.addEventListener("click", async () => {
  const file = movieInput.files[0];
  if (!file) return;

  uploadBtn.disabled = true;
  document.getElementById("upload-progress").classList.remove("hidden");

  const formData = new FormData();
  formData.append("video", file);

  try {
    const res = await fetch("/api/upload", { method: "POST", body: formData });
    const data = await res.json();

    hostToken = data.hostToken;
    sessionStorage.setItem("streamla_token", hostToken);
    isHost = true;

    startPlayer();
  } catch (err) {
    alert("Upload failed: " + err.message);
    uploadBtn.disabled = false;
  }
});

// 3. Join Watch Party
joinBtn.addEventListener("click", () => {
  // If stored token exists, verify host role
  if (hostToken) isHost = true;
  startPlayer();
});

// 4. Start Player & WebSockets
function startPlayer() {
  setupCard.classList.add("hidden");
  videoWrapper.classList.remove("hidden");
  roomBadge.classList.remove("hidden");
  viewersBadge.classList.remove("hidden");

  video.load(); // Load the /stream source

  if (isHost) {
    video.controls = true;
    roleTag.textContent = "Host Mode (Playback Controller)";
    setupHostListeners();
  } else {
    video.controls = false;
    roleTag.textContent = "Viewer Mode (Synced)";
  }

  connectWS();
}

// 5. WebSocket Connection & Sync
function connectWS() {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const url = `${proto}//${location.host}/ws?token=${encodeURIComponent(hostToken)}`;
  ws = new WebSocket(url);

  ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);

    if (msg.count !== undefined) {
      viewerCountSpan.textContent = msg.count;
    }

    if (isHost) return; // Host controls their own player

    // Viewers mirror the server state:
    isProgrammaticChange = true;

    // Correct drift if more than 1.5 seconds off
    const drift = Math.abs(video.currentTime - msg.position);
    if (drift > 1.5) {
      video.currentTime = msg.position;
    }

    if (msg.isPlaying && video.paused) {
      video.play().catch(() => {});
    } else if (!msg.isPlaying && !video.paused) {
      video.pause();
    }

    setTimeout(() => {
      isProgrammaticChange = false;
    }, 200);
  };

  // Periodic drift check every 5 seconds
  setInterval(() => {
    if (ws && ws.readyState === WebSocket.OPEN && !isHost) {
      ws.send(JSON.stringify({ type: "SYNC" }));
    }
  }, 5000);
}

// 6. Host Player Controls
function setupHostListeners() {
  video.addEventListener("play", () => {
    if (isProgrammaticChange || !ws) return;
    ws.send(
      JSON.stringify({
        type: "PLAY",
        position: video.currentTime,
        token: hostToken,
      }),
    );
  });

  video.addEventListener("pause", () => {
    if (isProgrammaticChange || !ws) return;
    ws.send(
      JSON.stringify({
        type: "PAUSE",
        position: video.currentTime,
        token: hostToken,
      }),
    );
  });

  video.addEventListener("seeked", () => {
    if (isProgrammaticChange || !ws) return;
    ws.send(
      JSON.stringify({
        type: "SEEK",
        position: video.currentTime,
        token: hostToken,
      }),
    );
  });
}

// 7. Share Link Copy
copyBtn.addEventListener("click", () => {
  navigator.clipboard.writeText(window.location.origin);
  copyBtn.textContent = "Done";
  setTimeout(() => {
    copyBtn.textContent = "Copy";
  }, 2000);
});

init();
