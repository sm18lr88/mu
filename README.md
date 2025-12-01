# mu

Personal app that provides essential online services without ads, algorithms, or tracking.
This fork is an experimental implementation of a papyrus-like theme, is local-first for testing purposes, and caters to a broader audience.
It's unstable and not production-ready.

<img width="6000" height="900" alt="image" src="https://github.com/user-attachments/assets/38450e3b-dcec-4b6e-b663-6e911ed34b62" />


Includes:

- **Chat** - AI-powered assistant with contextual discussions
- **Posts** - Microblogging and community sharing
- **News** - Curated RSS feeds and market data
- **Video** - YouTube search and viewing
- **API** - REST API for programmatic access

Mu runs as a single Go binary on your own server or use the hosted version at [mu.xyz](https://mu.xyz).

## Quick Start

1. Install Go 1.21+.
2. Clone the repo and install the binary:
   ```bash
   git clone https://github.com/sm18lr88/mu
   cd mu && go install
   ```
3. Run Mu (defaults to port 8030) and let it open your browser:
   ```bash
   mu --serve --address :8030
   ```
4. Visit http://localhost:8030/home. Data and cached content are stored locally under `$HOME/.mu`.

## Motivation

Big tech failed us. They now fuel addictive behaviour to drive profit above all else. The tools no longer work for us, instead we work for them.
Let's rebuild these services without ads, algorithms or exploits for a better way of life.

## Features

Starting with:

- [ ] Codex - use with ChatGPT subscription; avoid API $
- [x] API - Basic API
- [x] App - Basic PWA
- [x] Home - Overview
- [x] Chat - LLM chat UI
- [x] News - RSS news feed
- [x] Video - YouTube search
- [x] Posts - Micro blogging

Coming soon:

- [ ] Mail - Private inbox
- [ ] Wallet - Credits for usage
- [ ] Utilities - QR code scanner, etc
- [ ] Services - Marketplace of services

## Concepts

**Cards** displayed on the home screen are a sort of summary or overview. Each links to a **micro app** or an external website. For example the latest Video "more" links to the /video page with videos by channel and search, whereas the markets card redirects to an external app.

There are built in cards and then the idea would be that you could develop or include additional cards or micro apps through configuration or via some basic gist like code editor. Essentially creating a marketplace.

## Self Hosting

Ensure you have [Go](https://go.dev/doc/install) installed

Set your Go bin

```
export PATH=$HOME/go/bin:$PATH
```

Download and install Mu

```
git clone https://github.com/sm18lr88/mu
cd mu && go install
```

### Chat Prompts

Set the chat prompts in chat/prompts.json

### Home Cards

Set the home cards in home/cards.json

### News Feed

Set the RSS news feeds in news/feeds.json

### Video Channels

Set the YouTube video channels in video/channels.json

### API Keys

We need API keys for the following

#### Video Search

- [Youtube Data](https://developers.google.com/youtube/v3)

```
export YOUTUBE_API_KEY=xxx
```

#### LLM Model

Mu can use **OpenAI Codex CLI** as the backend for the Chat app without any extra API keys.

Requirements:

1. Install Codex CLI (one time):

   ```bash
   npm i -g @openai/codex
   ```

2. Log in with your ChatGPT account (Plus, Pro, Business, Edu, or Enterprise):

   ```bash
   codex login
   ```

Codex handles authentication and billing using your existing ChatGPT subscription; Mu does not need your API key.

Optional:

```bash
export MU_CHAT_BACKEND=codex
```

to force Codex even if other backends are configured. Fanar remains available as a fallback:

```bash
export FANAR_API_KEY=xxx
```

For vector search see this [doc](VECTOR_SEARCH.md)

### Run

Then run the app

```
mu --serve --address :8030
```

Go to http://localhost:8030/home (change the address with `--address` if you prefer another port).
