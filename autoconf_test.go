package main

import (
	"strings"
	"testing"

	autoconf "github.com/ipfs/boxo/autoconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetNativeSystems verifies that routing types are correctly mapped to native systems
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
			expectedSystems: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			systems := getNativeSystems(tt.routingType)
			assert.Equal(t, tt.expectedSystems, systems)
		})
	}
}

// TestExpandAutoBootstrap verifies bootstrap peer expansion behavior when autoconf is disabled
func TestExpandAutoBootstrap(t *testing.T) {
	cfg := Config{
		AutoConf: AutoConfConfig{
			Enabled: false,
		},
		DHTRouting: DHTStandard,
	}

	t.Run("auto placeholder errors when autoconf disabled", func(t *testing.T) {
		_, err := expandAutoBootstrap(autoconf.AutoPlaceholder, cfg, nil)
		require.Error(t, err, "should error when 'auto' is used with autoconf disabled")
		assert.Contains(t, err.Error(), "'auto' placeholder found in bootstrap peers", "error should mention bootstrap peers")
		assert.Contains(t, err.Error(), "RAINBOW_BOOTSTRAP", "error should mention how to fix")
	})

	t.Run("custom bootstrap preserved when autoconf disabled", func(t *testing.T) {
		result, err := expandAutoBootstrap("/ip4/127.0.0.1/tcp/4001/p2p/QmTest", cfg, nil)
		require.NoError(t, err, "custom bootstrap should not error")
		assert.Equal(t, []string{"/ip4/127.0.0.1/tcp/4001/p2p/QmTest"}, result, "custom bootstrap should be preserved")
	})

	t.Run("mixed auto and custom errors when autoconf disabled", func(t *testing.T) {
		_, err := expandAutoBootstrap(autoconf.AutoPlaceholder+",/ip4/127.0.0.1/tcp/4001/p2p/QmTest", cfg, nil)
		require.Error(t, err, "should error when 'auto' is mixed with custom values")
	})
}

// TestExpandAutoDNSResolvers verifies DNS resolver expansion behavior when autoconf is disabled
func TestExpandAutoDNSResolvers(t *testing.T) {
	cfg := Config{
		AutoConf: AutoConfConfig{
			Enabled: false,
		},
	}

	t.Run("auto placeholder errors when autoconf disabled", func(t *testing.T) {
		resolvers := []string{"eth.:" + autoconf.AutoPlaceholder}
		_, err := expandAutoDNSResolvers(resolvers, cfg, nil)
		require.Error(t, err, "should error when 'auto' is used with autoconf disabled")
		assert.Contains(t, err.Error(), "'auto' placeholder found in DNS resolvers", "error should mention DNS resolvers")
		assert.Contains(t, err.Error(), "RAINBOW_DNSLINK_RESOLVERS", "error should mention how to fix")
	})

	t.Run("custom resolver preserved when autoconf disabled", func(t *testing.T) {
		resolvers := []string{"example.com:https://dns.example.com/dns-query"}
		result, err := expandAutoDNSResolvers(resolvers, cfg, nil)
		require.NoError(t, err, "custom resolver should not error")
		expected := map[string]string{"example.com": "https://dns.example.com/dns-query"}
		assert.Equal(t, expected, result, "custom resolver should be preserved")
	})
}

// TestExpandAutoHTTPRouters verifies HTTP router expansion behavior when autoconf is disabled
func TestExpandAutoHTTPRouters(t *testing.T) {
	cfg := Config{
		AutoConf: AutoConfConfig{
			Enabled: false,
		},
		DHTRouting: DHTStandard,
	}

	t.Run("auto placeholder errors when autoconf disabled", func(t *testing.T) {
		routers := []string{autoconf.AutoPlaceholder}
		_, err := expandAutoHTTPRouters(routers, cfg, nil)
		require.Error(t, err, "should error when 'auto' is used with autoconf disabled")
		assert.Contains(t, err.Error(), "'auto' placeholder found in HTTP routers", "error should mention HTTP routers")
		assert.Contains(t, err.Error(), "RAINBOW_HTTP_ROUTERS", "error should mention how to fix")
	})

	t.Run("custom router preserved when autoconf disabled", func(t *testing.T) {
		routers := []string{"https://custom-router.com/routing/v1"}
		result, err := expandAutoHTTPRouters(routers, cfg, nil)
		require.NoError(t, err, "custom router should not error")
		assert.Equal(t, []string{"https://custom-router.com/routing/v1"}, result, "custom router should be preserved")
	})

	t.Run("mixed auto and custom errors when autoconf disabled", func(t *testing.T) {
		routers := []string{autoconf.AutoPlaceholder, "https://custom-router.com/routing/v1"}
		_, err := expandAutoHTTPRouters(routers, cfg, nil)
		require.Error(t, err, "should error when 'auto' is mixed with custom values")
	})
}

// TestDNSResolverWildcardBehavior verifies DNS resolver wildcard and auto placeholder handling
func TestDNSResolverWildcardBehavior(t *testing.T) {
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

			if !tt.autoconfEnabled {
				// Check if input contains "auto" placeholder
				hasAuto := false
				for _, input := range tt.input {
					if strings.Contains(input, autoconf.AutoPlaceholder) {
						hasAuto = true
						break
					}
				}

				if hasAuto {
					require.Error(t, err, tt.description+": should error with auto placeholder")
					return
				}

				require.NoError(t, err, tt.description)
				assert.Equal(t, tt.expectedValues, result, "parsing should produce expected result")
			} else {
				// When autoconf is enabled, check that custom values are preserved
				for domain, expectedURL := range tt.expectedValues {
					if domain != "." && expectedURL != "auto" {
						assert.Equal(t, expectedURL, result[domain], tt.description+": custom value for "+domain)
					}
				}
			}
		})
	}
}

// TestDNSResolverParsing verifies DNS resolver string parsing
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
			require.NoError(t, err, "parsing should not error")
			assert.Equal(t, tt.expected, result, "should parse to expected map")
		})
	}
}

// TestGetRoutingType verifies DHTRouting enum to string conversion
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
			assert.Equal(t, tt.expected, result)
		})
	}
}
