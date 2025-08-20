package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	autoconf "github.com/ipfs/boxo/autoconf"
)

// AutoConfConfig contains the configuration for the autoconf subsystem
type AutoConfConfig struct {
	// Enabled determines whether to use autoconf
	// Default: true
	Enabled bool

	// URL is the HTTP(S) URL to fetch the autoconf.json from
	// Default: https://conf.ipfs-mainnet.org/autoconf.json
	URL string

	// RefreshInterval is how often to refresh autoconf data
	// Default: 24h
	RefreshInterval time.Duration

	// CacheDir is the directory to cache autoconf data
	// Default: $RAINBOW_DATADIR/.autoconf-cache
	CacheDir string
}

// getNativeSystems returns the list of systems that should be used natively based on routing type
func getNativeSystems(routingType string) []string {
	switch routingType {
	case "dht", "accelerated", "standard", "auto":
		return []string{autoconf.SystemAminoDHT}
	case "off", "none", "custom":
		return []string{}
	default:
		goLog.Warnf("getNativeSystems: unknown routing type %q, assuming no native systems", routingType)
		return []string{}
	}
}

// getRoutingType converts DHTRouting enum to string routing type for autoconf
func getRoutingType(dhtRouting DHTRouting) string {
	switch dhtRouting {
	case DHTOff:
		return "off"
	case DHTStandard:
		return "standard"
	case DHTAccelerated:
		return "accelerated"
	default:
		// Any other value (including "custom") is treated as auto
		return "auto"
	}
}

// autoconfDisabledError returns a consistent error message when auto placeholder is found but autoconf is disabled
func autoconfDisabledError(configType, envVar, flag string) error {
	return fmt.Errorf("'auto' placeholder found in %s but autoconf is disabled. Set explicit %s with %s or %s, or re-enable autoconf",
		configType, configType, envVar, flag)
}

// expandAutoBootstrap expands "auto" placeholders in bootstrap peers
func expandAutoBootstrap(bootstrapStr string, cfg Config, autoConfData *autoconf.Config) ([]string, error) {
	if bootstrapStr == "" {
		return []string{}, nil
	}

	bootstrapList := strings.Split(bootstrapStr, ",")
	for i, s := range bootstrapList {
		bootstrapList[i] = strings.TrimSpace(s)
	}

	if !cfg.AutoConf.Enabled {
		if slices.Contains(bootstrapList, autoconf.AutoPlaceholder) {
			return nil, autoconfDisabledError("bootstrap peers", "RAINBOW_BOOTSTRAP", "--bootstrap")
		}
		return bootstrapList, nil
	}

	routingType := getRoutingType(cfg.DHTRouting)
	nativeSystems := getNativeSystems(routingType)
	return autoconf.ExpandBootstrapPeers(bootstrapList, autoConfData, nativeSystems), nil
}

// expandAutoDNSResolvers expands "auto" placeholders in DNS resolvers
func expandAutoDNSResolvers(resolversList []string, cfg Config, autoConfData *autoconf.Config) (map[string]string, error) {
	resolversMap := make(map[string]string, len(resolversList))
	for _, resolver := range resolversList {
		parts := strings.SplitN(resolver, ":", 2)
		if len(parts) == 2 {
			domain := strings.TrimSpace(parts[0])
			url := strings.TrimSpace(parts[1])
			resolversMap[domain] = url
		}
	}

	if !cfg.AutoConf.Enabled {
		for _, url := range resolversMap {
			if url == autoconf.AutoPlaceholder {
				return nil, autoconfDisabledError("DNS resolvers", "RAINBOW_DNSLINK_RESOLVERS", "--dnslink-resolvers")
			}
		}
		return resolversMap, nil
	}

	return autoconf.ExpandDNSResolvers(resolversMap, autoConfData), nil
}

// expandAutoHTTPRouters expands "auto" placeholders in HTTP routers
func expandAutoHTTPRouters(routers []string, cfg Config, autoConfData *autoconf.Config) ([]string, error) {
	if !cfg.AutoConf.Enabled {
		if slices.Contains(routers, autoconf.AutoPlaceholder) {
			return nil, autoconfDisabledError("HTTP routers", "RAINBOW_HTTP_ROUTERS", "--http-routers")
		}
		return routers, nil
	}

	routingType := getRoutingType(cfg.DHTRouting)
	nativeSystems := getNativeSystems(routingType)

	// Rainbow only uses read-only endpoints for providers, peers, and IPNS
	return autoconf.ExpandDelegatedEndpoints(routers, autoConfData, nativeSystems,
		autoconf.RoutingV1ProvidersPath,
		autoconf.RoutingV1PeersPath,
		autoconf.RoutingV1IPNSPath), nil
}

// createAutoConfClient creates an autoconf client with the given configuration
func createAutoConfClient(config AutoConfConfig) (*autoconf.Client, error) {
	if config.CacheDir == "" {
		config.CacheDir = filepath.Join(".", ".autoconf-cache")
	}
	if config.RefreshInterval == 0 {
		config.RefreshInterval = autoconf.DefaultRefreshInterval
	}
	if config.URL == "" {
		config.URL = autoconf.MainnetAutoConfURL
	}

	return autoconf.NewClient(
		autoconf.WithCacheDir(config.CacheDir),
		autoconf.WithUserAgent("rainbow/"+version),
		autoconf.WithCacheSize(autoconf.DefaultCacheSize),
		autoconf.WithTimeout(autoconf.DefaultTimeout),
		autoconf.WithURL(config.URL),
		autoconf.WithRefreshInterval(config.RefreshInterval),
	)
}
