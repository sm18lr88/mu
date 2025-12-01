package data

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// SaveFile saves data to disk
func SaveFile(key, val string) error {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	os.MkdirAll(path, 0700)
	os.WriteFile(file, []byte(val), 0644)
	return nil
}

// LoadFile loads a file from disk
func LoadFile(key string) ([]byte, error) {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	return os.ReadFile(file)
}

func SaveJSON(key string, val interface{}) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}

	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	os.MkdirAll(filepath.Dir(file), 0700)
	os.WriteFile(file, b, 0644)

	return nil
}

// LoadJSON loads JSON from disk into the provided struct pointer.
func LoadJSON(key string, val interface{}) error {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)

	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, val)
}

// ============================================
// SIMPLE INDEXING & SEARCH FOR RAG
// ============================================

var (
	indexMutex sync.RWMutex
	index      = make(map[string]*IndexEntry)

	embeddingCacheMu sync.RWMutex
	embeddingCache   = make(map[string][]float64)
)

// IndexEntry represents a searchable piece of content
type IndexEntry struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"` // "news", "video", "market", "reminder"
	Title         string                 `json:"title"`
	Content       string                 `json:"content"`
	Metadata      map[string]interface{} `json:"metadata"`
	Embedding     []float64              `json:"embedding"`      // Vector embedding for semantic search
	EmbeddingHash string                 `json:"embedding_hash"` // Hash of embedded text to avoid recompute
	IndexedAt     time.Time              `json:"indexed_at"`
}

// SearchResult represents a search hit with relevance score
type SearchResult struct {
	Entry *IndexEntry
	Score float64
}

// Index adds or updates an entry in the search index
func Index(id, entryType, title, content string, metadata map[string]interface{}) {
	indexMutex.RLock()
	existing := index[id]
	indexMutex.RUnlock()

	sameContent := false
	if existing != nil {
		sameContent = existing.Type == entryType &&
			existing.Title == title &&
			existing.Content == content &&
			reflect.DeepEqual(existing.Metadata, metadata)

		// If nothing changed and we already have an embedding, skip re-indexing
		if sameContent && len(existing.Embedding) > 0 {
			return
		}
	}

	entry := &IndexEntry{
		ID:        id,
		Type:      entryType,
		Title:     title,
		Content:   content,
		Metadata:  metadata,
		IndexedAt: time.Now(),
	}

	// Generate embedding for semantic search
	textToEmbed := title
	if len(content) > 0 {
		// Combine title and beginning of content for better embeddings
		maxContent := 500
		if len(content) < maxContent {
			maxContent = len(content)
		}
		textToEmbed = title + " " + content[:maxContent]
	}

	embedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(textToEmbed)))

	var embedding []float64

	// Reuse existing embedding if the embedded text hasn't changed
	if existing != nil && existing.EmbeddingHash == embedHash && len(existing.Embedding) > 0 {
		embedding = existing.Embedding
	} else {
		var err error
		embedding, err = getEmbedding(textToEmbed)
		if err != nil {
			embedding = nil
		}
	}

	if len(embedding) > 0 {
		entry.Embedding = embedding
		entry.EmbeddingHash = embedHash
	}

	indexMutex.Lock()
	index[id] = entry
	indexMutex.Unlock()

	// Persist to disk
	saveIndex()
}

// GetByID retrieves an entry by its exact ID
func GetByID(id string) *IndexEntry {
	indexMutex.RLock()
	defer indexMutex.RUnlock()
	return index[id]
}

// Search performs semantic vector search with keyword fallback
func Search(query string, limit int) []*IndexEntry {
	indexMutex.RLock()
	defer indexMutex.RUnlock()

	// Try vector search first
	queryLower := strings.ToLower(query)
	queryEmbedding, err := getEmbedding(query)
	if err == nil && len(queryEmbedding) > 0 {
		fmt.Printf("[SEARCH] Using vector search for: %s\n", query)
		var results []SearchResult

		for _, entry := range index {
			if len(entry.Embedding) == 0 {
				continue // Skip entries without embeddings
			}

			similarity := cosineSimilarity(queryEmbedding, entry.Embedding)
			if similarity > 0.3 {
				titleLower := strings.ToLower(entry.Title)
				contentLower := strings.ToLower(entry.Content)

				// Require either a text hit or a stronger semantic score
				if !strings.Contains(titleLower, queryLower) && !strings.Contains(contentLower, queryLower) && similarity < 0.6 {
					continue
				}

				results = append(results, SearchResult{Entry: entry, Score: similarity})
			}
		}

		// Sort by similarity descending
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})

		// Return top N results
		if limit > 0 && len(results) > limit {
			results = results[:limit]
		}

		entries := make([]*IndexEntry, len(results))
		for i, r := range results {
			entries[i] = r.Entry
		}

		if len(entries) > 0 {
			return entries
		}
	}

	// Fallback: Simple text matching if vector search fails or returns no results
	fmt.Printf("[SEARCH] Using text fallback for: %s\n", query)
	var results []SearchResult

	for _, entry := range index {
		score := 0.0
		titleLower := strings.ToLower(entry.Title)
		contentLower := strings.ToLower(entry.Content)

		// Simple contains matching
		if strings.Contains(titleLower, queryLower) {
			score = 3.0
		} else if strings.Contains(contentLower, queryLower) {
			score = 1.0
		}

		if score > 0 {
			results = append(results, SearchResult{
				Entry: entry,
				Score: score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Return top N results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	entries := make([]*IndexEntry, len(results))
	for i, r := range results {
		entries[i] = r.Entry
	}

	return entries
}

// GetByType returns all entries of a specific type
func GetByType(entryType string, limit int) []*IndexEntry {
	indexMutex.RLock()
	defer indexMutex.RUnlock()

	var entries []*IndexEntry
	for _, entry := range index {
		if entry.Type == entryType {
			entries = append(entries, entry)
		}
	}

	// Sort by indexed time descending (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].IndexedAt.After(entries[j].IndexedAt)
	})

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	return entries
}

// ClearIndex removes all entries from the index
func ClearIndex() {
	indexMutex.Lock()
	index = make(map[string]*IndexEntry)
	indexMutex.Unlock()
	saveIndex()
}

// saveIndex persists the index to disk
func saveIndex() {
	indexMutex.RLock()
	defer indexMutex.RUnlock()

	SaveJSON("index.json", index)
}

// Load loads the index from disk
func Load() {
	b, err := LoadFile("index.json")
	if err != nil {
		return
	}

	indexMutex.Lock()
	defer indexMutex.Unlock()

	json.Unmarshal(b, &index)

	// load embedding cache (best-effort)
	if cacheBytes, err := LoadFile("embedding_cache.json"); err == nil && len(cacheBytes) > 0 {
		var cache map[string][]float64
		if err := json.Unmarshal(cacheBytes, &cache); err == nil && cache != nil {
			embeddingCacheMu.Lock()
			embeddingCache = cache
			embeddingCacheMu.Unlock()
		}
	}
}

// ============================================
// VECTOR EMBEDDINGS VIA OLLAMA
// ============================================

const defaultEmbedModel = "qwen3-embedding:0.6b"

// getEmbedding generates a vector embedding for text using Ollama
func getEmbedding(text string) ([]float64, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("empty text")
	}

	// return cached embedding if available
	key := strings.TrimSpace(text)
	embeddingCacheMu.RLock()
	if cached, ok := embeddingCache[key]; ok && len(cached) > 0 {
		embeddingCacheMu.RUnlock()
		return cached, nil
	}
	embeddingCacheMu.RUnlock()

	fmt.Printf("[data] Generating embedding for text (length: %d)\n", len(text))

	// Ollama embedding endpoint
	url := "http://localhost:11434/api/embeddings"

	model := strings.TrimSpace(os.Getenv("OLLAMA_EMBED_MODEL"))
	if model == "" {
		model = defaultEmbedModel
	}

	requestBody := map[string]interface{}{
		"model":  model,
		"prompt": text,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error: %s", string(body))
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// persist to cache asynchronously (best-effort)
	embeddingCacheMu.Lock()
	embeddingCache[key] = result.Embedding
	embeddingCacheMu.Unlock()
	go func(snapshot map[string][]float64) {
		SaveJSON("embedding_cache.json", snapshot)
	}(copyEmbeddingCache())

	return result.Embedding, nil
}

// cosineSimilarity calculates cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64

	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func copyEmbeddingCache() map[string][]float64 {
	embeddingCacheMu.RLock()
	defer embeddingCacheMu.RUnlock()
	cp := make(map[string][]float64, len(embeddingCache))
	for k, v := range embeddingCache {
		cp[k] = v
	}
	return cp
}
