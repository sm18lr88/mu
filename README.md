# mu

Local-first, ad-free personal hub. This fork experiments with a papyrus theme and is not production-ready.

<img width="885" height="610" alt="image" src="https://github.com/user-attachments/assets/6a1a2ad5-39d2-42b1-b0b1-8dd2dbff451f" />

Support app author by using the free hosted version at [mu.xyz](https://mu.xyz)

## Features

- Chat (LLM), Posts, RSS News + markets, Video search, REST API
- PWA home cards linking to micro apps or external tools
- Coming soon: Use with ChatGPT subscription rather than API, Mail, Wallet, Utilities, Services

## Quick Start

1. Install Go 1.21+ (`export PATH=$HOME/go/bin:$PATH`).
2. Clone and install:
   ```bash
   git clone https://github.com/sm18lr88/mu
   cd mu && go install
   ```
3. Run (defaults to :8030 and opens browser):
   ```bash
   mu --serve --address :8030
   ```
4. Open http://localhost:8030/home. Data lives in `$HOME/.mu`.

## Configure

- Chat prompts: `chat/prompts.json`
- Home cards: `home/cards.json`
- RSS feeds: `news/feeds.json`
- Video channels: `video/channels.json`
- Vector search: see `VECTOR_SEARCH.md`

## API Keys

- Video search: create a [YouTube Data](https://developers.google.com/youtube/v3) key and `export YOUTUBE_API_KEY=xxx`.
- Chat backend: uses **OpenAI Codex CLI** (no extra API key). Install and log in once:
  ```bash
  npm i -g @openai/codex
  codex login
  ```
  Optional: `export MU_CHAT_BACKEND=codex` to force Codex; `export FANAR_API_KEY=xxx` for the Fanar fallback.

## Motivation

Rebuild everyday services without ads, tracking, or engagement tricks—so the tools work for you.

## Additional screenshots

<img width="1445" height="891" alt="image" src="https://github.com/user-attachments/assets/367d643b-0fb2-41d6-804c-974426bba2d7" />

<img width="658" height="940" alt="image" src="https://github.com/user-attachments/assets/7c3b1f85-f777-481d-a623-e4dcd5896954" />
