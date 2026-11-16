# Streamla — Product Requirements Document

## 1. Summary

Streamla is a self-hosted movie streaming app for local networks. One person hosts a movie from their own machine; friends on the same Wi-Fi/LAN open a link in their browser and watch it together with synchronized playback. No cloud, no accounts, no internet dependency.

## 2. Goals

- Host picks one movie file and streams it to other devices on the LAN.
- All viewers see the same moment of the movie, controlled by the host.
- Joining is as easy as opening a link or scanning a QR code.
- Runs from a single binary with a browser UI. No install on viewer devices.

## 3. Non-Goals (v0.1)

- Media libraries, directory scanning, metadata, posters
- Accounts, chat, reactions, history, recommendations
- Transcoding (FFmpeg), HLS, adaptive bitrate, subtitles, multiple audio tracks
- Internet/cloud/peer-to-peer streaming, public exposure
- Database, desktop app, Docker packaging
- Native in-app server discovery (see 5.4)

## 4. Users

- **Host:** runs Streamla on their machine, picks the movie, controls playback.
- **Viewer:** joins from any modern browser on the same network. Cannot control playback.

## 5. Functional Requirements

### 5.1 Movie Selection

- Host picks a single video file through a file picker and sees its filename.
- Host can replace the selected movie.
- The file is uploaded to the server and stored temporarily on disk, never fully held in memory.
- Supported format: MP4 with browser-compatible codecs (H.264/AAC). The file is validated on upload, including whether it is seekable/progressively playable.
- Upload size limit is configurable.
- Temporary files are deleted when the session ends or the server shuts down.

### 5.2 Streaming

- Movie is served over HTTP with byte-range support (206 Partial Content), enabling seeking, pause/resume, and per-viewer buffering.
- Multiple viewers stream the same file concurrently, each over an independent connection.
- Streaming is file-backed; client disconnects release resources cleanly.

### 5.3 Watch Party

- One server runs one session with one room and one active movie.
- The host creates the room and receives a secret host token. The room has an unguessable ID.
- Viewers join via a shareable address (URL/QR code) or by entering the room ID. They can leave at any time.
- The UI shows the participant count to everyone, and the participant list to the host.
- Host controls: play, pause, seek, end the party. Viewers have no playback controls.
- Server holds the authoritative playback state (position, playing/paused) in memory.
- Playback events: PLAY, PAUSE, SEEK, SYNC, JOIN, LEAVE, delivered over WebSockets.
- Late joiners and reconnecting viewers receive the current state immediately.
- Clients periodically resync: small drift is corrected gradually (playback-rate adjustment), large drift triggers a seek. Clients estimate their clock offset to the server so latency is accounted for.
- Viewers must perform one click (e.g. "Join") before playback, to satisfy browser autoplay policy.
- Only messages carrying a valid host token may issue playback control events; all incoming messages are validated.
- Disconnected clients are removed from room state. The room closes when the host ends the party or the host stays disconnected beyond a grace period.

### 5.4 Connecting & Discovery

Browsers cannot browse mDNS, so discovery works as follows:

- On startup, the server prints its LAN URL and a QR code in the terminal and shows the shareable address in the host UI.
- The server also advertises itself via mDNS (`_streamla._tcp`) and a `.local` hostname, for clients and OSes that can resolve it. Advertising stops on shutdown.
- Manual IP:port entry always works and is the primary fallback.
- Known limitation: networks with multicast blocking or Wi-Fi client isolation will break mDNS and may break LAN access entirely.

## 6. User Flow

1. Host starts Streamla and opens the UI.
2. Host selects a movie and creates the room.
3. Host shares the link/QR code.
4. Viewers open it and join.
5. Host presses play; everyone watches in sync.

## 7. Screens

- **Home:** branding, connection status, create party, join by address/room ID.
- **Host:** select movie, filename, create room, room ID and share link, participant list, player with controls.
- **Join:** room ID entry, join button.
- **Watch room:** large player, filename, room ID, participant count, host indicator, connection status, host-only controls.

Design: plain HTML/CSS/JS, dark minimal theme, responsive, player is the focal element, no chat or social elements.

## 8. Non-Functional Requirements

- **Architecture:** single Go server serving the frontend, API, streams, and WebSocket traffic. In-memory state, no database.
- **Performance:** several simultaneous viewers on a typical home LAN; lightweight WebSocket messages; no full-file memory loads; no CPU-heavy work.
- **Reliability:** graceful handling of client drops and server shutdown; concurrency-safe room state; no goroutine or file-handle leaks; temp file cleanup.
- **Security:** upload validation and size limits; no arbitrary filesystem access or path traversal; unguessable room IDs; host-token-restricted controls; connection limits; LAN-only by default (not bound to public interfaces).
- **Compatibility:** Linux host, IPv4 LAN, modern browsers.

## 9. Known Limitations

- MP4 only. MKV, HEVC, and AC3-audio files will not play without transcoding, which is out of scope.
- Uploading duplicates the file on the host's disk.
- Synchronization targets correctness over frame-accurate precision.
- One room per server.

## 10. Acceptance Criteria

- [ ] Server starts, serves the UI, and prints a LAN URL and QR code.
- [ ] Host selects an MP4; it is stored temporarily and removed on session end or shutdown.
- [ ] A second device on the LAN can open the link and play the movie, with working seeking.
- [ ] Multiple viewers stream concurrently without issues.
- [ ] Viewers can join and leave; participant count is accurate.
- [ ] Host play/pause/seek propagates to all viewers; viewers cannot control playback.
- [ ] Late joiners and reconnecting viewers land at the current state.
- [ ] Viewers stay in sync within a reasonable tolerance on LAN, with drift corrected automatically.
- [ ] Manual address entry works; mDNS hostname resolves where the network allows.
- [ ] Runs with no cloud or internet dependency and passes Go race detection.

## 11. Success Statement

Streamla does four things reliably: select, stream, synchronize, connect.