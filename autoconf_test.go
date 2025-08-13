package main

import (
	"testing"

	autoconf "github.com/ipfs/boxo/autoconf"
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

	// Test auto placeholder with autoconf disabled - should be stripped
	result := expandAutoBootstrap(autoconf.AutoPlaceholder, cfg, nil)
	if len(result) != 0 {
		t.Errorf("Expected empty list when 'auto' is used with autoconf disabled, got %v", result)
	}

	// Test custom bootstrap with autoconf disabled
	result = expandAutoBootstrap("/ip4/127.0.0.1/tcp/4001/p2p/QmTest", cfg, nil)
	if len(result) != 1 || result[0] != "/ip4/127.0.0.1/tcp/4001/p2p/QmTest" {
		t.Errorf("Expected custom bootstrap to be preserved, got %v", result)
	}

	// Test comma-separated values - auto should be stripped
	result = expandAutoBootstrap(autoconf.AutoPlaceholder+",/ip4/127.0.0.1/tcp/4001/p2p/QmTest", cfg, nil)
	if len(result) != 1 || result[0] != "/ip4/127.0.0.1/tcp/4001/p2p/QmTest" {
		t.Errorf("Expected only custom bootstrap when autoconf disabled, got %v", result)
	}
}

func TestExpandAutoDNSResolvers(t *testing.T) {
	cfg := Config{
		AutoConf: AutoConfConfig{
			Enabled: false,
		},
	}

	// Test auto placeholder with autoconf disabled - should be stripped
	resolvers := []string{"eth.:" + autoconf.AutoPlaceholder}
	result := expandAutoDNSResolvers(resolvers, cfg, nil)
	if len(result) != 0 {
		t.Errorf("Expected empty map when 'auto' is used with autoconf disabled, got %v", result)
	}

	// Test custom resolver
	resolvers = []string{"example.com:https://dns.example.com/dns-query"}
	result = expandAutoDNSResolvers(resolvers, cfg, nil)
	if len(result) != 1 || result["example.com"] != "https://dns.example.com/dns-query" {
		t.Errorf("Expected custom resolver to be preserved, got %v", result)
	}
}

func TestExpandAutoHTTPRouters(t *testing.T) {
	cfg := Config{
		AutoConf: AutoConfConfig{
			Enabled: false,
		},
		DHTRouting: DHTStandard,
	}

	// Test auto placeholder with autoconf disabled - should be stripped
	routers := []string{autoconf.AutoPlaceholder}
	result := expandAutoHTTPRouters(routers, cfg, nil)
	if len(result) != 0 {
		t.Errorf("Expected empty list when 'auto' is used with autoconf disabled, got %v", result)
	}

	// Test custom router
	routers = []string{"https://custom-router.com/routing/v1"}
	result = expandAutoHTTPRouters(routers, cfg, nil)
	if len(result) != 1 || result[0] != "https://custom-router.com/routing/v1" {
		t.Errorf("Expected custom router to be preserved, got %v", result)
	}

	// Test mixed auto and custom - auto should be stripped
	routers = []string{autoconf.AutoPlaceholder, "https://custom-router.com/routing/v1"}
	result = expandAutoHTTPRouters(routers, cfg, nil)
	if len(result) != 1 || result[0] != "https://custom-router.com/routing/v1" {
		t.Errorf("Expected only custom router when autoconf disabled, got %v", result)
	}
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

			result := expandAutoDNSResolvers(tt.input, cfg, nil)

			// When autoconf is disabled, check exact match
			if !tt.autoconfEnabled {
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
				if tt.input != nil && len(tt.input) > 0 && tt.input[0] != ". : auto" {
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
		{
			name:     "wildcard_domain",
			input:    []string{". : auto"},
			expected: map[string]string{}, // auto is stripped when autoconf is disabled
		},
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

			result := expandAutoDNSResolvers(tt.input, cfg, nil)

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
