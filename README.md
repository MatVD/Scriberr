<div align="center">

<img alt="SynthezIA" src="synthezia-black.png" width="480" />

Self‑hostable, secure & private offline transcription. Drop in a recording, get clean transcripts, highlight key moments, take notes or chat with your audio using your favorite LLM — all without sending your data to the cloud.

[Website](https://synthezia.app) • [Docs](https://synthezia.app/docs/intro.html) • [API Reference](https://synthezia.app/api.html) • [Changelog](https://synthezia.app/changelog.html)

<p align="center">
<a href='https://ko-fi.com/H2H41KQZA3' target='_blank'><img height='36' style='border:0px;height:36px;' src='https://storage.ko-fi.com/cdn/kofi6.png?v=6' border='0' alt='Buy Me a Coffee at ko-fi.com' /></a>
</p>
</div>

**Collecting feedback on new feature. Drop by https://github.com/rishikanthc/SynthezIA/discussions/200 to share your opinions.**


# Introduction

SynthezIA is a self‑hosted offline transcription app for converting audio into text. Record or upload audio, get it transcribed, and quickly summarize or chat using your preferred LLM provider. SynthezIA runs on modern CPUs (no GPU required, though GPUs can accelerate processing) and offers a range of trade‑offs between speed and transcription quality.

- Built with React (frontend) and Go (backend), packaged as a single binary
- Uses WhisperX with open‑source Whisper models for accurate transcription
- Clean, distraction‑free UI optimized for reading and working with transcripts

<p align="center">
  <img alt="SynthezIA homepage" src="screenshots/synthezia-homepage.png" width="720" />
</p>

## Features

- Accurate transcription with word‑level timing
- Speaker diarization (identify and label speakers)
- Transcript reader with playback follow‑along and seek‑from‑text
- Highlights and lightweight note‑taking (jump note → audio/transcript)
- Summarize and chat over transcripts (OpenAI or local models via Ollama)
- Transcription profiles for re‑usable configurations
- YouTube video transcription (paste a link and transcribe)
- Quick transcribe (ephemeral) and batch upload
- REST API coverage for all major features + API key management
- Download transcripts as JSON/SRT/TXT (and more)
- Support for Nvidia GPUs [New - Experimental]

## Screenshots

<details>
  <summary>Show screenshots</summary>

  <p align="center">
    <img alt="Transcript view" src="screenshots/synthezia-transcript page.png" width="720" />
  </p>
  <p align="center"><em>Minimal transcript reader with playback follow‑along and seek‑from‑text.</em></p>

  <p align="center">
    <img alt="Summarize transcripts" src="screenshots/synthezia-summarize transcripts.png" width="720" />
  </p>
  <p align="center"><em>Summarize long recordings and use custom prompts.</em></p>

  <p align="center">
    <img alt="API key management" src="screenshots/synthezia-api-key-management.png" width="720" />
  </p>
  <p align="center"><em>Generate and manage API keys for the REST API.</em></p>

  <p align="center">
    <img alt="YouTube video transcription" src="screenshots/synthezia-youtube-video.png" width="720" />
  </p>
  <p align="center"><em>Transcribe audio directly from a YouTube link.</em></p>

</details>

## Installation

Visit the website for the full guide: https://synthezia.app/docs/installation.html

### Homebrew (macOS & Linux)

```bash
brew tap rishikanthc/synthezia
brew install synthezia

# Start the server
synthezia
```

Open http://localhost:8080 in your browser.

Optional configuration via .env (sensible defaults provided):

```env
# Server
HOST=localhost
PORT=8080

# Storage
DATABASE_PATH=./data/synthezia.db
UPLOAD_DIR=./data/uploads
WHISPERX_ENV=./data/whisperx-env

# Custom paths (if needed)
UV_PATH=/custom/path/to/uv
```

### Docker

Run the command below in a shell:

```bash
docker run -d \
  --name synthezia \
  -p 8080:8080 \
  -v synthezia_data:/app/data \
  --restart unless-stopped \
  ghcr.io/rishikanthc/synthezia:latest
```

#### Docker Compose:

```yaml
version: '3.9'
services:
  synthezia:
    image: ghcr.io/rishikanthc/synthezia:latest
    container_name: synthezia
    ports:
      - "8080:8080"
    volumes:
      - synthezia_data:/app/data
    restart: unless-stopped

volumes:
  synthezia_data:
```

#### With GPU (CUDA)
```yaml
version: "3.9"
services:
  synthezia:
    image: ghcr.io/rishikanthc/synthezia:v1.0.4-cuda
    ports:
      - "8080:8080"
    volumes:
      - synthezia_data:/app/data
    restart: unless-stopped
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities:
                - gpu
    environment:
      - NVIDIA_VISIBLE_DEVICES=all
      - NVIDIA_DRIVER_CAPABILITIES=compute,utility

volumes:
  synthezia_data: {}
```

Then open http://localhost:8080.

## Diarization (speaker identification)

SynthezIA uses the open‑source pyannote models for local speaker diarization. Models are hosted on Hugging Face and require an access token (only used to download models — diarization runs locally).

1) Create an account on https://huggingface.co

2) Visit and accept the user conditions for these repositories:
   - https://huggingface.co/pyannote/speaker-diarization-3.0
   - https://huggingface.co/pyannote/speaker-diarization
   - https://huggingface.co/pyannote/speaker-diarization-3.1
   - https://huggingface.co/pyannote/segmentation-3.0

   Verify they appear here: https://huggingface.co/settings/gated-repos

3) Create an access token under Settings → Access Tokens and enable all permissions under “Repositories”. Keep it safe.

4) In SynthezIA, when creating a profile or using Transcribe+, open the Diarization tab and paste the token into the “Hugging Face Token” field.

See the full guide: https://synthezia.app/docs/diarization.html

<p align="center">
  <img alt="Diarization setup" src="screenshots/synthezia-diarization-setup.png" width="420" />
</p>

## API

SynthezIA exposes a clean REST API for most features (transcription, chat, notes, summaries, admin, and more). Authentication supports JWT or API keys depending on endpoint.

- API Reference: https://synthezia.app/api.html
- Quick start examples (cURL and JS) on the API page
- Generate or manage API keys in the app

## Contributing

Issues and PRs are welcome. Please open an issue to discuss large changes first and keep PRs focused.

Local dev overview:

```bash
# Backend (dev)
cp -n .env.example .env || true
go run cmd/server/main.go

# Frontend (dev)
cd web/frontend
npm ci
npm run dev

# Full build (embeds UI in Go binary)
./build.sh
./synthezia
```

Coding style: `go fmt ./...`, `go vet ./...`, and `cd web/frontend && npm run lint`.

## Donating

<a href='https://ko-fi.com/H2H41KQZA3' target='_blank'><img height='36' style='border:0px;height:36px;' src='https://storage.ko-fi.com/cdn/kofi6.png?v=6' border='0' alt='Buy Me a Coffee at ko-fi.com' /></a>
