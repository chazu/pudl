package importer

import (
	"testing"
)

func TestDetectCollectionWrapper_PositiveCases(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		wantKey  string
		wantLen  int
		minScore float64
	}{
		{
			name: "simple wrapper with known key and homogeneous items",
			data: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": float64(1), "name": "a"},
					map[string]interface{}{"id": float64(2), "name": "b"},
				},
			},
			wantKey:  "items",
			wantLen:  2,
			minScore: 0.50,
		},
		{
			name: "wrapper with count matching array length",
			data: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": float64(1)},
					map[string]interface{}{"id": float64(2)},
				},
				"count": float64(2),
			},
			wantKey:  "items",
			wantLen:  2,
			minScore: 0.50,
		},
		{
			name: "wrapper with pagination signals",
			data: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"name": "a"},
				},
				"next_token": "abc",
				"total":      float64(1),
			},
			wantKey:  "data",
			wantLen:  1,
			minScore: 0.50,
		},
		{
			name: "envelope pattern with meta and links",
			data: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"x": float64(1)},
				},
				"meta":  map[string]interface{}{"page": float64(1)},
				"links": map[string]interface{}{"next": "/p2"},
			},
			wantKey:  "data",
			wantLen:  1,
			minScore: 0.50,
		},
		{
			name: "stripe-like response",
			data: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"id": "ch_1", "amount": float64(100)},
				},
				"has_more": true,
				"url":      "/charges",
			},
			wantKey:  "data",
			wantLen:  1,
			minScore: 0.50,
		},
		{
			name: "AWS DynamoDB case-insensitive key",
			data: map[string]interface{}{
				"Items": []interface{}{
					map[string]interface{}{"pk": map[string]interface{}{"S": "1"}},
				},
				"Count":        float64(1),
				"ScannedCount": float64(1),
			},
			wantKey:  "Items",
			wantLen:  1,
			minScore: 0.50,
		},
		{
			name: "large homogeneous array",
			data: func() map[string]interface{} {
				items := make([]interface{}, 50)
				for i := range items {
					items[i] = map[string]interface{}{
						"id":   float64(i),
						"name": "item",
						"type": "widget",
					}
				}
				return map[string]interface{}{
					"results": items,
					"total":   float64(50),
				}
			}(),
			wantKey:  "results",
			wantLen:  50,
			minScore: 0.50,
		},
		{
			name: "elasticsearch hits",
			data: map[string]interface{}{
				"hits": []interface{}{
					map[string]interface{}{"_id": "1", "_source": map[string]interface{}{"title": "doc"}},
				},
				"total": map[string]interface{}{"value": float64(1)},
			},
			wantKey:  "hits",
			wantLen:  1,
			minScore: 0.50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectCollectionWrapper(tt.data)
			if result == nil {
				t.Fatal("expected wrapper detection, got nil")
			}
			if result.ArrayKey != tt.wantKey {
				t.Errorf("ArrayKey = %q, want %q", result.ArrayKey, tt.wantKey)
			}
			if len(result.Items) != tt.wantLen {
				t.Errorf("len(Items) = %d, want %d", len(result.Items), tt.wantLen)
			}
			if result.Score < tt.minScore {
				t.Errorf("Score = %.2f, want >= %.2f", result.Score, tt.minScore)
			}
			if len(result.Signals) == 0 {
				t.Error("expected at least one signal")
			}
		})
	}
}

func TestDetectCollectionWrapper_NegativeCases(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{
			name: "resource with tags (known attribute key + primitive array)",
			data: map[string]interface{}{
				"name": "server-1",
				"tags": []interface{}{"web", "prod"},
				"id":   "srv-1",
			},
		},
		{
			name: "resource with data field but too many scalar fields",
			data: map[string]interface{}{
				"data":    []interface{}{map[string]interface{}{"x": float64(1)}},
				"name":    "config",
				"version": "1.0",
				"type":    "widget",
				"author":  "joe",
				"created": "2024",
				"updated": "2024",
				"id":      "cfg-1",
			},
		},
		{
			name: "multiple similar arrays penalty",
			data: map[string]interface{}{
				"users":  []interface{}{map[string]interface{}{"id": float64(1)}},
				"roles":  []interface{}{map[string]interface{}{"id": float64(1)}},
				"groups": []interface{}{map[string]interface{}{"id": float64(1)}},
			},
		},
		{
			name: "primitive array elements",
			data: map[string]interface{}{
				"items": []interface{}{float64(1), float64(2), float64(3)},
				"count": float64(3),
			},
		},
		{
			name: "empty array",
			data: map[string]interface{}{
				"items": []interface{}{},
				"count": float64(0),
			},
		},
		{
			name: "empty map input",
			data: map[string]interface{}{},
		},
		{
			name: "resource with nested array and many scalar fields",
			data: map[string]interface{}{
				"name":      "menu",
				"items":     []interface{}{map[string]interface{}{"label": "File"}},
				"parent_id": float64(3),
				"type":      "nav",
				"icon":      "menu",
				"position":  float64(5),
				"visible":   true,
				"id":        "menu-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectCollectionWrapper(tt.data)
			if result != nil {
				t.Errorf("expected nil, got detection with key=%q score=%.2f signals=%v",
					result.ArrayKey, result.Score, result.Signals)
			}
		})
	}
}

func TestDetectCollectionWrapper_EdgeCases(t *testing.T) {
	t.Run("exactly at threshold should detect", func(t *testing.T) {
		// known wrapper key (+0.35) + homogeneous (+0.15) + few keys (+0.05) = 0.55
		// This should be above threshold.
		data := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": float64(1)},
				map[string]interface{}{"id": float64(2)},
			},
		}
		result := DetectCollectionWrapper(data)
		if result == nil {
			t.Fatal("expected detection at/above threshold")
		}
		if result.Score < wrapperScoreThreshold {
			t.Errorf("Score = %.2f, want >= %.2f", result.Score, wrapperScoreThreshold)
		}
	})

	t.Run("just below threshold should not detect", func(t *testing.T) {
		// Unknown key name, no pagination, no count, single element (homogeneous trivially)
		// but no known wrapper key = only homogeneous (+0.15) + few keys (+0.05) + dominant (+0.05) = 0.25
		data := map[string]interface{}{
			"widgets": []interface{}{
				map[string]interface{}{"id": float64(1)},
			},
		}
		result := DetectCollectionWrapper(data)
		if result != nil {
			t.Errorf("expected nil below threshold, got score=%.2f signals=%v",
				result.Score, result.Signals)
		}
	})

	t.Run("single element array with known key and pagination", func(t *testing.T) {
		data := map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"id": "only-one"},
			},
			"has_more": false,
			"total":    float64(1),
		}
		result := DetectCollectionWrapper(data)
		if result == nil {
			t.Fatal("expected detection for single-element with pagination")
		}
		if result.ArrayKey != "data" {
			t.Errorf("ArrayKey = %q, want %q", result.ArrayKey, "data")
		}
	})

	t.Run("unknown key with strong pagination and count signals", func(t *testing.T) {
		// No known wrapper key, but pagination (+0.25) + count match (+0.20) +
		// homogeneous (+0.15) + few keys (+0.05) + dominant (+0.05) = 0.70
		data := map[string]interface{}{
			"things": []interface{}{
				map[string]interface{}{"id": float64(1), "val": "a"},
				map[string]interface{}{"id": float64(2), "val": "b"},
				map[string]interface{}{"id": float64(3), "val": "c"},
			},
			"total":    float64(3),
			"has_more": false,
		}
		result := DetectCollectionWrapper(data)
		if result == nil {
			t.Fatal("expected detection from pagination + count signals")
		}
		if result.Score < 0.50 {
			t.Errorf("Score = %.2f, want >= 0.50", result.Score)
		}
	})
}

func TestDetectCollectionWrapper_WrapperMeta(t *testing.T) {
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"id": float64(1)},
		},
		"count":    float64(1),
		"has_more": false,
	}
	result := DetectCollectionWrapper(data)
	if result == nil {
		t.Fatal("expected detection")
	}
	if _, ok := result.WrapperMeta["count"]; !ok {
		t.Error("WrapperMeta should contain 'count'")
	}
	if _, ok := result.WrapperMeta["has_more"]; !ok {
		t.Error("WrapperMeta should contain 'has_more'")
	}
	if _, ok := result.WrapperMeta["items"]; ok {
		t.Error("WrapperMeta should NOT contain 'items' (the array key)")
	}
}

func TestDetectCollectionWrapper_BestCandidateWins(t *testing.T) {
	// Two array fields: "items" (known key) should win over "misc" (unknown)
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"id": float64(1)},
			map[string]interface{}{"id": float64(2)},
		},
		"misc": []interface{}{
			map[string]interface{}{"x": float64(1)},
		},
		"count": float64(2),
	}
	result := DetectCollectionWrapper(data)
	if result == nil {
		t.Fatal("expected detection")
	}
	if result.ArrayKey != "items" {
		t.Errorf("ArrayKey = %q, want %q (best candidate)", result.ArrayKey, "items")
	}
}
