package data

import (
	"math"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	tmpDir, _ := os.MkdirTemp("", "mu_test_data")
	originalHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)

	code := m.Run()

	_ = os.Setenv("HOME", originalHome)
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    []float64
		b    []float64
		want float64
	}{
		{"Identical", []float64{1, 0}, []float64{1, 0}, 1.0},
		{"Orthogonal", []float64{1, 0}, []float64{0, 1}, 0.0},
		{"Opposite", []float64{1}, []float64{-1}, -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("cosineSimilarity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIndexingAndFallbackSearch(t *testing.T) {
	ClearIndex()

	Index("1", "news", "Bitcoin hits new high", "Crypto markets are rallying today.", nil)
	Index("2", "news", "Apple releases new iPhone", "Tech giant reveals latest gadget.", nil)

	item := GetByID("1")
	if item == nil || item.Title != "Bitcoin hits new high" {
		t.Error("GetByID failed")
	}

	results := Search("Bitcoin", 10)
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'Bitcoin', got %d", len(results))
	} else if results[0].ID != "1" {
		t.Error("Search returned wrong item")
	}

	results = Search("iPhone", 10)
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'iPhone', got %d", len(results))
	} else if results[0].ID != "2" {
		t.Error("Search returned wrong item")
	}
}
