package main

import (
	"strings"
	"testing"

	autoconf "github.com/ipfs/boxo/autoconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNativeSystems(t *testing.T) {
	tests := []struct {
		name            string
		routingType     string
		expectedSystems []string
	}{
		{
			name:            "DHT routing",
			routingType:     "dht",
			expectedSystems: []string{autoconf.SystemAminoDHT},
		},
		{
			name:            "Accelerated routing",
			routingType:     "accelerated",
			expectedSystems: []string{autoconf.SystemAminoDHT},
		},
		{
			name:            "Standard routing",
			routingType:     "standard",
			expectedSystems: []string{autoconf.SystemAminoDHT},
		},
		{
			name:            "Auto routing",
			routingType:     "auto",
			expectedSystems: []string{autoconf.SystemAminoDHT},
		},
		{
			name:            "Off routing",
			routingType:     "off",
			expectedSystems: []string{},
		},
		{
			name:            "None routing",
			routingType:     "none",
			expectedSystems: []string{},
		},
		{
			name:            "Unknown routing type",
			routingType:     "custom",
			expectedSystems: []string{}, // Now returns empty for unknown types
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			systems := getNativeSystems(tt.routingType)
			if len(systems) != len(tt.expectedSystems) {
				t.Errorf("getNativeSystems(%q) returned %d systems, expected %d",
					tt.routingType, len(systems), len(tt.expectedSystems))
			}
			for i, sys := range systems {
				if i < len(tt.expectedSystems) && sys != tt.expectedSystems[i] {
					t.Errorf("getNativeSystems(%q)[%d] = %q, expected %q",
						tt.routingType, i, sys, tt.expectedSystems[i])
				}
			}
		})
	}
}

func TestExpandAutoBootstrap(t *testing.T) {
	cfg := Config{
		AutoConf: AutoConfConfig{
			Enabled: false,
		},
		DHTRouting: DHTStandard,
	}

	// Test auto placeholder with autoconf disabled - should error
	_, err := expandAutoBootstrap(autoconf.AutoPlaceholder, cfg, nil)
	require.Error(t, err, "Expected error when 'auto' is used with autoconf disabled")
	assert.Contains(t, err.Error(), "'auto' placeholder found in bootstrap peers but autoconf is disabled")
	assert.Contains(t, err.Error(), "RAINBOW_BOOTSTRAP")

	// Test custom bootstrap with autoconf disabled
	result, err := expandAutoBootstrap("/ip4/127.0.0.1/tcp/4001/p2p/QmTest", cfg, nil)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "/ip4/127.0.0.1/tcp/4001/p2p/QmTest", result[0])

	// Test comma-separated values - auto should cause error
	_, err = expandAutoBootstrap(autoconf.AutoPlaceholder+",/ip4/127.0.0.1/tcp/4001/p2p/QmTest", cfg, nil)
	require.Error(t, err, "Expected error when 'auto' is mixed with custom bootstrap")
}

func TestExpandAutoDNSResolvers(t *testing.T) {
	cfg := Config{
		AutoConf: AutoConfConfig{
			Enabled: false,
		},
	}

	// Test auto placeholder with autoconf disabled - should error
	resolvers := []string{"eth.:" + autoconf.AutoPlaceholder}
	_, err := expandAutoDNSResolvers(resolvers, cfg, nil)
	require.Error(t, err, "Expected error when 'auto' is used with autoconf disabled")
	assert.Contains(t, err.Error(), "'auto' placeholder found in DNS resolvers but autoconf is disabled")
	assert.Contains(t, err.Error(), "RAINBOW_DNSLINK_RESOLVERS")

	// Test custom resolver
	resolvers = []string{"example.com:https://dns.example.com/dns-query"}
	result, err := expandAutoDNSResolvers(resolvers, cfg, nil)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "https://dns.example.com/dns-query", result["example.com"])
}

func TestExpandAutoHTTPRouters(t *testing.T) {
	cfg := Config{
		AutoConf: AutoConfConfig{
			Enabled: false,
		},
		DHTRouting: DHTStandard,
	}

	// Test auto placeholder with autoconf disabled - should error
	routers := []string{autoconf.AutoPlaceholder}
	_, err := expandAutoHTTPRouters(routers, cfg, nil)
	require.Error(t, err, "Expected error when 'auto' is used with autoconf disabled")
	assert.Contains(t, err.Error(), "'auto' placeholder found in HTTP routers but autoconf is disabled")
	assert.Contains(t, err.Error(), "RAINBOW_HTTP_ROUTERS")

	// Test custom router
	routers = []string{"https://custom-router.com/routing/v1"}
	result, err := expandAutoHTTPRouters(routers, cfg, nil)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "https://custom-router.com/routing/v1", result[0])

	// Test mixed auto and custom - auto should error
	routers = []string{autoconf.AutoPlaceholder, "https://custom-router.com/routing/v1"}
	_, err = expandAutoHTTPRouters(routers, cfg, nil)
	require.Error(t, err, "Expected error when 'auto' is mixed with custom router")
}

func TestDNSResolverWildcardBehavior(t *testing.T) {
	// Test cases for DNS resolver wildcard behavior
	tests := []struct {
		name            string
		input           []string
		autoconfEnabled bool
		expectedKeys    []string
		expectedValues  map[string]string
		description     string
	}{
		{
			name:            "wildcard_auto_imports_all",
			input:           []string{". : auto"},
			autoconfEnabled: true,
			expectedKeys:    []string{}, // Will be populated by autoconf
			expectedValues: map[string]string{
				".": "auto", // When autoconf disabled, kept as-is
			},
			description: "Wildcard '.': 'auto' should import all autoconf resolvers",
		},
		{
			name: "wildcard_auto_with_custom_override",
			input: []string{
				". : auto",
				"eth. : https://custom.eth.resolver/dns-query",
			},
			autoconfEnabled: true,
			expectedKeys:    []string{"eth."}, // eth. should be present with custom value
			expectedValues: map[string]string{
				"eth.": "https://custom.eth.resolver/dns-query",
			},
			description: "User-provided eth. should override autoconf value",
		},
		{
			name: "no_wildcard_only_explicit",
			input: []string{
				"eth. : https://my.resolver/dns-query",
			},
			autoconfEnabled: true,
			expectedKeys:    []string{"eth."},
			expectedValues: map[string]string{
				"eth.": "https://my.resolver/dns-query",
			},
			description: "Without wildcard, only explicit resolvers should be used",
		},
		{
			name: "specific_domain_auto",
			input: []string{
				"crypto. : auto",
			},
			autoconfEnabled: true,
			expectedKeys:    []string{"crypto."},
			expectedValues: map[string]string{
				"crypto.": "auto", // Will expand if autoconf has crypto.
			},
			description: "Specific domain with 'auto' should only expand that domain",
		},
		{
			name: "mixed_specific_and_custom",
			input: []string{
				"crypto. : auto",
				"eth. : https://custom.eth.resolver/dns-query",
			},
			autoconfEnabled: true,
			expectedKeys:    []string{"crypto.", "eth."},
			expectedValues: map[string]string{
				"eth.":    "https://custom.eth.resolver/dns-query",
				"crypto.": "auto", // Will expand if autoconf has crypto.
			},
			description: "Mix of auto and custom without wildcard",
		},
		{
			name:            "wildcard_auto_disabled",
			input:           []string{". : auto"},
			autoconfEnabled: false,
			expectedKeys:    []string{},
			expectedValues:  map[string]string{},
			description:     "Wildcard with autoconf disabled should strip 'auto'",
		},
		{
			name:            "empty_input",
			input:           []string{},
			autoconfEnabled: true,
			expectedKeys:    []string{},
			expectedValues:  map[string]string{},
			description:     "Empty input should result in empty output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				AutoConf: AutoConfConfig{
					Enabled: tt.autoconfEnabled,
				},
			}

			result, err := expandAutoDNSResolvers(tt.input, cfg, nil)

			// When autoconf is disabled, check exact match
			if !tt.autoconfEnabled {
				// Special case: if input contains "auto", expect an error
				hasAuto := false
				for _, input := range tt.input {
					if strings.Contains(input, autoconf.AutoPlaceholder) {
						hasAuto = true
						break
					}
				}
				if hasAuto {
					if err == nil {
						t.Errorf("%s: expected error when 'auto' is used with autoconf disabled", tt.description)
					}
					return // Skip further checks if we expect an error
				}
				if err != nil {
					t.Errorf("%s: unexpected error: %v", tt.description, err)
					return
				}
				if len(result) != len(tt.expectedValues) {
					t.Errorf("%s: expected %d resolvers, got %d", tt.description, len(tt.expectedValues), len(result))
				}
				for domain, expectedURL := range tt.expectedValues {
					if actualURL, exists := result[domain]; !exists || actualURL != expectedURL {
						t.Errorf("%s: expected %s -> %s, got %v (exists: %v)", tt.description, domain, expectedURL, actualURL, exists)
					}
				}
			} else {
				// When autoconf is enabled, check specific expectations
				// For wildcard cases, we can't predict all keys without actual autoconf data
				// But we can check that user overrides are respected
				for domain, expectedURL := range tt.expectedValues {
					if domain != "." && expectedURL != "auto" {
						// Check that custom values are preserved
						if actualURL, exists := result[domain]; !exists || actualURL != expectedURL {
							t.Errorf("%s: expected custom %s -> %s, got %s", tt.description, domain, expectedURL, actualURL)
						}
					}
				}

				// For non-wildcard cases, check that only expected keys exist
				if len(tt.input) > 0 && tt.input[0] != ". : auto" {
					// Count non-auto entries
					expectedCount := 0
					for _, expectedURL := range tt.expectedValues {
						if expectedURL != "auto" {
							expectedCount++
						}
					}
					if expectedCount > 0 && len(result) > expectedCount {
						t.Errorf("%s: without wildcard, should only have explicit resolvers", tt.description)
					}
				}
			}
		})
	}
}

func TestDNSResolverParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected map[string]string
	}{
		{
			name:     "simple_domain",
			input:    []string{"example.com : https://dns.example.com/dns-query"},
			expected: map[string]string{"example.com": "https://dns.example.com/dns-query"},
		},
		// Note: "auto" placeholder test is covered in TestExpandAutoDNSResolvers
		{
			name: "multiple_domains",
			input: []string{
				"eth. : https://eth.resolver/dns-query",
				"crypto. : https://crypto.resolver/dns-query",
			},
			expected: map[string]string{
				"eth.":    "https://eth.resolver/dns-query",
				"crypto.": "https://crypto.resolver/dns-query",
			},
		},
		{
			name:     "spaces_around_separator",
			input:    []string{"  eth.  :  https://eth.resolver/dns-query  "},
			expected: map[string]string{"eth.": "https://eth.resolver/dns-query"},
		},
		{
			name:     "no_separator",
			input:    []string{"invalid-no-separator"},
			expected: map[string]string{},
		},
		{
			name:     "empty_string",
			input:    []string{""},
			expected: map[string]string{},
		},
		{
			name:     "url_with_colon",
			input:    []string{"example. : https://dns.example.com:8080/dns-query"},
			expected: map[string]string{"example.": "https://dns.example.com:8080/dns-query"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				AutoConf: AutoConfConfig{
					Enabled: false, // Disable autoconf to test parsing only
				},
			}

			result, err := expandAutoDNSResolvers(tt.input, cfg, nil)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d resolvers, got %d", len(tt.expected), len(result))
			}

			for domain, expectedURL := range tt.expected {
				if actualURL, exists := result[domain]; !exists || actualURL != expectedURL {
					t.Errorf("Expected %s -> %s, got %s", domain, expectedURL, actualURL)
				}
			}
		})
	}
}
