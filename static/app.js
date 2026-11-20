let isHost = false;
let hostToken = sessionStorage.getItem("streamla_token") || "";
let ws = null;
let isProgrammaticChange = false; // prevents echo loops
let wakeLock = null;

const video = document.getElementById("player");
const setupCard = document.getElementById("setup-card");
const videoWrapper = document.getElementById("video-wrapper");
const movieStatus = document.getElementById("movie-status");
const movieInput = document.getElementById("movie-input");
const uploadBtn = document.getElementById("upload-btn");
const fileNameSpan = document.getElementById("file-name");
const joinBtn = document.getElementById("join-btn");
const joinRoomInput = document.getElementById("join-room-input");
const roomBadge = document.getElementById("room-badge");
const roomIdSpan = document.getElementById("room-id");
const viewersBadge = document.getElementById("viewers-badge");
const viewerCountSpan = document.getElementById("viewer-count");
const syncBadge = document.getElementById("sync-badge");
const syncStatusSpan = document.getElementById("sync-status");
const roleTag = document.getElementById("role-tag");
const copyBtn = document.getElementById("copy-btn");

// Check query param for host token on initial load
const urlParams = new URLSearchParams(window.location.search);
if (urlParams.get("token")) {
  hostToken = urlParams.get("token");
  sessionStorage.setItem("streamla_token", hostToken);
  isHost = true;
}

// 1. Initial State Check
async function init() {
  try {
    const res = await fetch("/api/info");
    const info = await res.json();
    roomIdSpan.textContent = info.roomId;
    if (joinRoomInput) joinRoomInput.value = info.roomId;

    if (info.hasMovie) {
      movieStatus.textContent = `Movie: ${info.filename}`;
    } else {
      movieStatus.textContent =
        "No movie loaded yet. Ready for viewers to join.";
    }
  } catch (err) {
    movieStatus.textContent = "Unable to connect to server.";
  }
}

// 2. File Selection & Upload (Host UI fallback)
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
    if (!res.ok) {
      const errText = await res.text();
      throw new Error(errText);
    }
    const data = await res.json();

    hostToken = data.hostToken;
    sessionStorage.setItem("streamla_token", hostToken);
    isHost = true;

    startPlayer();
  } catch (err) {
    alert("Upload failed: " + err.message);
    uploadBtn.disabled = false;
    document.getElementById("upload-progress").classList.add("hidden");
  }
});

// 3. Screen Wake Lock (prevents mobile display sleep)
async function requestWakeLock() {
  if ("wakeLock" in navigator) {
    try {
      wakeLock = await navigator.wakeLock.request("screen");
    } catch (err) {
      console.log("WakeLock error:", err);
    }
  }
}

document.addEventListener("visibilitychange", () => {
  if (wakeLock !== null && document.visibilityState === "visible") {
    requestWakeLock();
  }
});

// 4. Join Watch Party & Autoplay Unlock
joinBtn.addEventListener("click", () => {
  if (hostToken) isHost = true;
  startPlayer();
});

function startPlayer() {
  setupCard.classList.add("hidden");
  videoWrapper.classList.remove("hidden");
  roomBadge.classList.remove("hidden");
  viewersBadge.classList.remove("hidden");
  syncBadge.classList.remove("hidden");

  // Request Wake Lock
  requestWakeLock();

  // Attach stream endpoint
  video.src = "/stream";
  video.load();

  // Autoplay unlock gesture
  video
    .play()
    .then(() => {
      // Audio session successfully unlocked
      if (!isHost) {
        video.pause();
      }
    })
    .catch(() => {});

  if (isHost) {
    video.controls = true;
    roleTag.textContent = "Host Mode (Playback Controller)";
    if (syncStatusSpan) syncStatusSpan.textContent = "Host (Authoritative)";
    setupHostListeners();
  } else {
    video.controls = false;
    roleTag.textContent = "Viewer Mode (Synced)";
  }

  connectWS();
}

// 5. NTP Clock Synchronization (Cristian's Algorithm)
let clockOffset = 0; // serverTime - clientTime (ms)
let minRTT = Infinity;
let pingInterval = null;

function sendPing() {
  if (ws && ws.readyState === WebSocket.OPEN) {
    const t1 = performance.timeOrigin + performance.now();
    ws.send(JSON.stringify({ type: "PING", t1: t1 }));
  }
}

function handlePong(msg) {
  const t4 = performance.timeOrigin + performance.now();
  const rtt = (t4 - msg.t1) - (msg.t3 - msg.t2);
  const offset = (msg.t2 - msg.t1 + (msg.t3 - t4)) / 2;

  // Track the minimum RTT sample for least jitter
  if (rtt < minRTT || minRTT === Infinity) {
    minRTT = rtt;
    clockOffset = offset;
  }
}

function estimatedServerTime() {
  return performance.timeOrigin + performance.now() + clockOffset;
}

function startClockSync() {
  minRTT = Infinity;
  // Burst of 5 pings for immediate calibration
  for (let i = 0; i < 5; i++) {
    setTimeout(sendPing, i * 80);
  }
  // Ongoing clock maintenance ping every 15s
  if (pingInterval) clearInterval(pingInterval);
  pingInterval = setInterval(sendPing, 15000);
}

// 6. Drift Correction Engine
let scheduledResumeTimer = null;
let lastKnownState = { position: 0, isPlaying: false, serverTime: 0 };

function updateSyncUI(delta) {
  if (!syncStatusSpan || isHost) return;
  const absDeltaMs = Math.round(Math.abs(delta) * 1000);
  if (absDeltaMs < 50) {
    syncStatusSpan.textContent = `Sync: Locked (${absDeltaMs}ms)`;
    syncStatusSpan.style.color = "#4ade80"; // green
  } else if (absDeltaMs <= 1000) {
    const dir = delta > 0 ? "behind" : "ahead";
    syncStatusSpan.textContent = `Sync: Nudging (${absDeltaMs}ms ${dir})`;
    syncStatusSpan.style.color = "#facc15"; // yellow
  } else {
    syncStatusSpan.textContent = `Sync: Catching up (${(absDeltaMs / 1000).toFixed(1)}s)`;
    syncStatusSpan.style.color = "#f87171"; // red
  }
}

function correctDrift(targetPos, isPlaying) {
  if (isProgrammaticChange || !video.src || video.readyState < 2) return;

  const currentPos = video.currentTime;
  const delta = targetPos - currentPos; // > 0 means viewer is lagging behind target

  updateSyncUI(delta);

  if (!isPlaying) {
    if (Math.abs(delta) > 0.05) {
      isProgrammaticChange = true;
      video.currentTime = targetPos;
      setTimeout(() => {
        isProgrammaticChange = false;
      }, 200);
    }
    video.playbackRate = 1.0;
    return;
  }

  // Zone 1: Deadband (< 50ms)
  if (Math.abs(delta) < 0.05) {
    if (video.playbackRate !== 1.0) {
      video.playbackRate = 1.0;
    }
    return;
  }

  // Zone 2: Micro-rate nudging (50ms - 1000ms)
  if (Math.abs(delta) <= 1.0) {
    if (delta > 0) {
      video.playbackRate = 1.05; // Gently speed up
    } else {
      video.playbackRate = 0.95; // Gently slow down
    }
    return;
  }

  // Zone 3: Hard seek (> 1000ms)
  isProgrammaticChange = true;
  video.currentTime = targetPos;
  video.playbackRate = 1.0;
  setTimeout(() => {
    isProgrammaticChange = false;
  }, 300);
}

// 7. WebSocket Signaling & Event Handling
function connectWS() {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const url = `${proto}//${location.host}/ws?token=${encodeURIComponent(hostToken)}`;
  ws = new WebSocket(url);

  ws.onopen = () => {
    startClockSync();
  };

  ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);

    if (msg.count !== undefined) {
      viewerCountSpan.textContent = msg.count;
    }

    switch (msg.type) {
      case "PONG":
        handlePong(msg);
        break;

      case "MOVIE_CHANGED":
        video.src = "/stream?t=" + Date.now();
        video.load();
        video.currentTime = 0;
        video.pause();
        break;

      case "PREPARE_PLAY":
      case "PREPARE_SEEK":
        lastKnownState = {
          position: msg.position,
          isPlaying: true,
          serverTime: msg.serverTime || estimatedServerTime(),
        };

        if (scheduledResumeTimer) {
          clearTimeout(scheduledResumeTimer);
          scheduledResumeTimer = null;
        }

        isProgrammaticChange = true;
        video.currentTime = msg.position;
        if (!video.paused) {
          video.pause();
        }

        const delayMs = msg.resumeAt - estimatedServerTime();
        const executePlay = () => {
          isProgrammaticChange = true;
          video.playbackRate = 1.0;
          video
            .play()
            .catch((err) => {
              console.warn("Scheduled play error:", err);
            })
            .finally(() => {
              setTimeout(() => {
                isProgrammaticChange = false;
              }, 250);
            });
        };

        if (delayMs > 0) {
          scheduledResumeTimer = setTimeout(executePlay, delayMs);
        } else {
          executePlay();
        }
        break;

      case "PAUSE":
        lastKnownState = {
          position: msg.position,
          isPlaying: false,
          serverTime: msg.serverTime || estimatedServerTime(),
        };

        if (scheduledResumeTimer) {
          clearTimeout(scheduledResumeTimer);
          scheduledResumeTimer = null;
        }

        if (isHost) return;

        isProgrammaticChange = true;
        if (!video.paused) video.pause();
        video.currentTime = msg.position;
        video.playbackRate = 1.0;
        setTimeout(() => {
          isProgrammaticChange = false;
        }, 200);
        break;

      case "SEEK":
        lastKnownState = {
          position: msg.position,
          isPlaying: false,
          serverTime: msg.serverTime || estimatedServerTime(),
        };

        if (isHost) return;

        isProgrammaticChange = true;
        video.currentTime = msg.position;
        if (!video.paused) video.pause();
        video.playbackRate = 1.0;
        setTimeout(() => {
          isProgrammaticChange = false;
        }, 200);
        break;

      case "SYNC":
      case "HEARTBEAT":
        lastKnownState = {
          position: msg.position,
          isPlaying: msg.isPlaying,
          serverTime: msg.serverTime || estimatedServerTime(),
        };

        if (isHost) return;

        // Calculate expected target position based on server time
        let expected = msg.position;
        if (msg.isPlaying && msg.serverTime) {
          const elapsed = (estimatedServerTime() - msg.serverTime) / 1000.0;
          expected += Math.max(0, elapsed);
        }

        if (msg.isPlaying && video.paused) {
          isProgrammaticChange = true;
          video.currentTime = expected;
          video
            .play()
            .catch(() => {})
            .finally(() => {
              setTimeout(() => {
                isProgrammaticChange = false;
              }, 250);
            });
        } else if (!msg.isPlaying && !video.paused) {
          isProgrammaticChange = true;
          video.pause();
          video.currentTime = expected;
          setTimeout(() => {
            isProgrammaticChange = false;
          }, 200);
        } else {
          correctDrift(expected, msg.isPlaying);
        }
        break;
    }
  };
}

// 8. Continuous Drift Monitoring Ticker (250ms)
setInterval(() => {
  if (!ws || ws.readyState !== WebSocket.OPEN || isHost) return;
  if (lastKnownState.isPlaying && lastKnownState.serverTime) {
    const elapsed = (estimatedServerTime() - lastKnownState.serverTime) / 1000.0;
    const target = lastKnownState.position + Math.max(0, elapsed);
    correctDrift(target, true);
  }
}, 250);

// 9. Host Player Controls
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

// 10. Share Link Copy
copyBtn.addEventListener("click", () => {
  navigator.clipboard.writeText(window.location.origin);
  copyBtn.textContent = "Done";
  setTimeout(() => {
    copyBtn.textContent = "Copy";
  }, 2000);
});

init();
