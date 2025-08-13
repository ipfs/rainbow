package main

import (
	"testing"
)

func TestGetRoutingType(t *testing.T) {
	tests := []struct {
		name     string
		input    DHTRouting
		expected string
	}{
		{
			name:     "DHT Off",
			input:    DHTOff,
			expected: "off",
		},
		{
			name:     "DHT Standard",
			input:    DHTStandard,
			expected: "standard",
		},
		{
			name:     "DHT Accelerated",
			input:    DHTAccelerated,
			expected: "accelerated",
		},
		{
			name:     "Default case",
			input:    DHTRouting("unknown"),
			expected: "auto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRoutingType(tt.input)
			if result != tt.expected {
				t.Errorf("getRoutingType(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}
