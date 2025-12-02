package data

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkSearchFallback(b *testing.B) {
	b.ReportAllocs()
	disableEmbeddings("benchmark")
	ClearIndex()

	body := strings.Repeat("body of content about bitcoin, markets, and macro trends. ", 3)
	for i := 0; i < 5000; i++ {
		Index(fmt.Sprintf("id-%d", i), "news", fmt.Sprintf("Title %d bitcoin", i%50), body, nil)
	}
	FlushIndex()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Search("bitcoin", 3)
	}
}

func BenchmarkIndexing(b *testing.B) {
	b.ReportAllocs()
	disableEmbeddings("benchmark")
	ClearIndex()

	body := strings.Repeat("cached metadata and links for quick indexing. ", 2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Index(fmt.Sprintf("id-%d", i), "news", fmt.Sprintf("Headline %d", i), body, nil)
	}
	FlushIndex()
}
