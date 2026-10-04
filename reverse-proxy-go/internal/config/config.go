package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type HealthCheckConfig struct {
	Path       string `json:"path"`
	IntervalMs int    `json:"intervalMs"`
}

type MatchConfig struct {
	Host       string `json:"host"`
	PathPrefix string `json:"pathPrefix"`
}

type RouteConfig struct {
	Match                 MatchConfig        `json:"match"`
	Upstream              string             `json:"upstream"`
	StripPrefix           bool               `json:"stripPrefix"`
	AddRequestHeaders     map[string]string  `json:"addRequestHeaders"`
	RemoveRequestHeaders  []string           `json:"removeRequestHeaders"`
	AddResponseHeaders    map[string]string  `json:"addResponseHeaders"`
	RemoveResponseHeaders []string           `json:"removeResponseHeaders"`
	HealthCheck           *HealthCheckConfig `json:"healthCheck"`
}

type ProxyConfig struct {
	ListenAddr string        `json:"listenAddr"`
	Routes     []RouteConfig `json:"routes"`
}

func LoadConfig(filePath string) (*ProxyConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ProxyConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if config.ListenAddr == "" {
		config.ListenAddr = ":8080"
	}

	if len(config.Routes) == 0 {
		return nil, fmt.Errorf("no routes configured")
	}

	for i, route := range config.Routes {
		if route.Upstream == "" {
			return nil, fmt.Errorf("route %d: upstream is required", i)
		}
		if route.Match.Host == "" && route.Match.PathPrefix == "" {
			return nil, fmt.Errorf("route %d: at least one of host or pathPrefix must be specified", i)
		}
	}

	return &config, nil
}

func DefaultConfig() *ProxyConfig {
	return &ProxyConfig{
		ListenAddr: ":8080",
		Routes: []RouteConfig{
			{
				Match: MatchConfig{
					PathPrefix: "/api",
				},
				Upstream:    "http://localhost:3000",
				StripPrefix: true,
			},
		},
	}
}
