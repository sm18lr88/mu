# Vector Search Setup

## Installation on DigitalOcean VPS

### 1. Install Ollama

```bash
curl -fsSL https://ollama.com/install.sh | sh
```

### 2. Pull the embedding model

```bash
ollama pull qwen3-embedding:0.6b
```

This downloads the `qwen3-embedding:0.6b` model (~520MB). It will start automatically when needed.

### 3. Verify Ollama is running

```bash
curl http://localhost:11434/api/embeddings -d '{
  "model": "qwen3-embedding:0.6b",
  "prompt": "test"
}'
```

Should return a JSON with an "embedding" array of **1024 floats**.

### 4. Restart your mu application

```bash
./mu
```

The app will now automatically:

- Generate embeddings for all indexed content (news, tickers, videos)
- Use semantic vector search for queries
- Fallback to keyword search if Ollama is unavailable

## How it works

- **Indexing**: When news/tickers are indexed, embeddings are generated automatically
- **Search**: Queries are embedded and compared using cosine similarity
- **Performance**: ~100-200ms per embedding on 1-2 CPU cores
- **Fallback**: If Ollama is down, keyword search is used automatically
- **Model**: Defaults to `qwen3-embedding:0.6b`; override with `OLLAMA_EMBED_MODEL`

## Testing

Try asking:

- "what's the bitcoin price" -> should find BTC ticker
- "ethereum value" -> should find ETH ticker
- "crypto markets" -> should find crypto-related news
- "digital gold" -> should find Bitcoin content

## Memory usage

- Ollama idle: ~600MB
- During embedding: +200MB temporarily
- Index with embeddings: ~6KB per entry (due to larger 1024 dimensions)
