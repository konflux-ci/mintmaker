package slices

import (
	"testing"
)

func TestFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		filter   func(int) bool
		expected []int
	}{
		{
			name:     "filter even numbers",
			input:    []int{1, 2, 3, 4, 5, 6},
			filter:   func(n int) bool { return n%2 == 0 },
			expected: []int{2, 4, 6},
		},
		{
			name:     "filter none matching",
			input:    []int{1, 3, 5},
			filter:   func(n int) bool { return n%2 == 0 },
			expected: nil,
		},
		{
			name:     "filter all matching",
			input:    []int{2, 4, 6},
			filter:   func(n int) bool { return n%2 == 0 },
			expected: []int{2, 4, 6},
		},
		{
			name:     "empty slice",
			input:    []int{},
			filter:   func(n int) bool { return true },
			expected: nil,
		},
		{
			name:     "nil slice",
			input:    nil,
			filter:   func(n int) bool { return true },
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Filter(tc.input, tc.filter)
			if len(result) != len(tc.expected) {
				t.Fatalf("expected length %d, got %d", len(tc.expected), len(result))
			}
			for i := range result {
				if result[i] != tc.expected[i] {
					t.Errorf("at index %d: expected %d, got %d", i, tc.expected[i], result[i])
				}
			}
		})
	}
}

func TestFilterStrings(t *testing.T) {
	result := Filter([]string{"hello", "world", "hi"}, func(s string) bool {
		return len(s) > 2
	})
	expected := []string{"hello", "world"}
	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("at index %d: expected %q, got %q", i, expected[i], result[i])
		}
	}
}

func TestIntersection(t *testing.T) {
	tests := []struct {
		name     string
		s1       []string
		s2       []string
		expected int
	}{
		{
			name:     "matching prefix",
			s1:       []string{"a", "b", "c"},
			s2:       []string{"a", "b", "d"},
			expected: 2,
		},
		{
			name:     "no ordered match",
			s1:       []string{"a", "b", "c"},
			s2:       []string{"b", "a", "c"},
			expected: 0,
		},
		{
			name:     "full match",
			s1:       []string{"a", "b", "c"},
			s2:       []string{"a", "b", "c"},
			expected: 3,
		},
		{
			name:     "empty slices",
			s1:       []string{},
			s2:       []string{},
			expected: 0,
		},
		{
			name:     "first empty",
			s1:       []string{},
			s2:       []string{"a", "b"},
			expected: 0,
		},
		{
			name:     "second empty",
			s1:       []string{"a", "b"},
			s2:       []string{},
			expected: 0,
		},
		{
			name:     "s1 longer than s2",
			s1:       []string{"a", "b", "c", "d"},
			s2:       []string{"a", "b"},
			expected: 2,
		},
		{
			name:     "s2 longer than s1",
			s1:       []string{"a", "b"},
			s2:       []string{"a", "b", "c", "d"},
			expected: 2,
		},
		{
			name:     "first element differs",
			s1:       []string{"x", "b", "c"},
			s2:       []string{"a", "b", "c"},
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Intersection(tc.s1, tc.s2)
			if result != tc.expected {
				t.Errorf("Intersection(%v, %v) = %d, expected %d", tc.s1, tc.s2, result, tc.expected)
			}
		})
	}
}
