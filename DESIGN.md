# Design

Mu is a social platform built to avoid the addiction loops of legacy networks.

## Principles

- Remove ad-driven incentives: no algorithmic feeds, likes, or rage-bait loops.
- Prioritize reflection over reaction: feeds curated by locally-hosted platform that surface your posts and nearby voices.
- Keep content useful: longform over shorts; curated news and video without clickbait.
- Ground AI in explicit ethics and global cultural context.

## Architecture

- Single Go codebase; monolithic app for speed and iteration.
- Local file storage for now; single-process server is acceptable long term.
- JSON-based MUCP protocol (if needed) instead of ActivityPub; avoids federation quirks and supports payments.

## Building Blocks

- Core surfaces: Chat, News, Video, Posts; Wallet/marketplace later.
- Provide auth and a simple API first; expand to services/agents invoked via chat.

## Future Work

- Add economic layer and services marketplace when the social fabric is stable.
