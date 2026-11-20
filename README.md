# Streamla

Streamla is a self-hosted local area network (LAN) video streaming server with synchronized playback. It allows a host to stream a video file to multiple browser clients on the same network without external internet access or third-party client software.

## Requirements

- Go 1.20 or newer
- FFmpeg and FFprobe installed and available on `PATH`

## Installation

Clone the repository and build the binary:

```bash
git clone https://github.com/Dharshan2208/streamla.git
cd streamla
go build -o streamla .
```

## Usage

Start the server by passing the path to a video file:

```bash
./streamla /path/to/movie.mp4
```

Alternatively, run directly with Go:

```bash
go run . /path/to/movie.mp4
```

### Flags

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `-port` | string | `6969` | Port for the HTTP server to listen on. |
| `-no-remux` | bool | `false` | Disable automatic FastStart remuxing for MP4 files. |

### Joining a Session

1. The host opens `http://localhost:6969` in a web browser and clicks "Start Watching".
2. Viewers on the same network navigate to `http://<host-ip>:6969` or scan the terminal QR code, then click "Join Watch Party".
3. Playback, pausing, and seeking performed by the host synchronize across all connected viewers via WebSockets.

## Project Structure

- `internal/discovery/`: Network interface detection and QR code generation.
- `internal/media/`: Video file inspection, FastStart atom validation, remuxing, subtitle conversion, and media streaming handlers.
- `internal/room/`: WebSocket connection handling, room state management, and playback synchronization.
- `static/`: Frontend assets (HTML, CSS, JavaScript) embedded into the Go binary at compile time.

## Development

Run tests:

```bash
go test ./...
```

Run tests with data race detection:

```bash
go test -race ./...
```

Run static analysis:

```bash
go vet ./...
```
