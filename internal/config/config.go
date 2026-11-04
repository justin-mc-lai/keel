// Package config assembles the provider registry from built-in presets,
// an optional user config file, and environment variables.
//
// Secrets live only in the environment or in a gitignored local file. Nothing
// in this package ever writes a key to disk inside the repo.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/justin-mc-lai/keel/internal/mcp"
	"github.com/justin-mc-lai/keel/internal/provider"
)

// Preset is a vendor definition shipped with the binary. API keys are never
// stored here, only the environment variable that holds them.
type Preset struct {
	Name          string
	BaseURL       string
	APIKeyEnv     string
	SessionHeader string
	NoProxy       bool
	DefaultModel  string
	Models        []string
}

// Presets is the out-of-the-box vendor list.
var Presets = []Preset{
	{
		Name:          "opencode-go",
		BaseURL:       "https://opencode.ai/zen/go/v1",
		APIKeyEnv:     "OPENCODE_GO_API_KEY",
		SessionHeader: "x-opencode-session",
		DefaultModel:  "deepseek-v4.1-flash",
		Models: []string{
			"deepseek-v4.1-flash", "deepseek-v4-pro", "glm-5.3", "glm-5.3-flash",
			"kimi-k3", "kimi-k2.7-code", "minimax-m3", "qwen3.8-max", "grok-4.7",
		},
	},
	{
		Name:         "commandcode",
		BaseURL:      "https://api.commandcode.ai/provider/v1",
		APIKeyEnv:    "COMMAND_CODE_API_KEY",
		NoProxy:      true,
		DefaultModel: "deepseek/deepseek-v4.1-flash",
		Models: []string{
			"deepseek/deepseek-v4.1-flash", "deepseek/deepseek-v4-pro",
			"z-ai/glm-5.3-flash", "Qwen/Qwen3.8-Max",
			"moonshotai/Kimi-K2.7-Code", "MiniMaxAI/MiniMax-M3",
			"gpt-5.6-luna", "google/gemini-3.8-flash",
		},
	},
}

// File is the optional user config. It can add vendors or override a preset.
type File struct {
	DefaultProvider string                      `json:"defaultProvider"`
	DefaultApproval string                      `json:"defaultApproval"`
	WorkspaceRoot   string                      `json:"workspaceRoot"`
	Providers       []FileProvider              `json:"providers"`
	MCPServers      map[string]mcp.ServerConfig `json:"mcpServers,omitempty"`
}

// FileProvider is one provider entry in the user config.
type FileProvider struct {
	Name          string            `json:"name"`
	BaseURL       string            `json:"baseURL"`
	APIKey        string            `json:"apiKey"`
	APIKeyEnv     string            `json:"apiKeyEnv"`
	Headers       map[string]string `json:"headers"`
	SessionHeader string            `json:"sessionHeader"`
	NoProxy       bool              `json:"noProxy"`
	DefaultModel  string            `json:"defaultModel"`
	Models        []string          `json:"models"`
}

// Path returns the user config location, honouring KEEL_CONFIG.
func Path() string {
	if v := os.Getenv("KEEL_CONFIG"); v != "" {
		return v
	}
	if v := os.Getenv("KEEL_STATE_DIR"); v != "" {
		return filepath.Join(v, "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".keel", "config.json")
}

// LoadFile reads the raw user config file, returning an empty value when it
// does not exist.
func LoadFile() (File, error) {
	path := Path()
	if path == "" {
		return File{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, nil
		}
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return File{}, fmt.Errorf("config %s: %w", path, err)
	}
	return f, nil
}

// Load builds the registry from presets, then the user config file, then the
// environment.
func Load() (*provider.Registry, error) {
	LoadDotEnv()
	reg := provider.NewRegistry()

	for _, p := range Presets {
		reg.Add(presetProvider(p, nil))
	}

	path := Path()
	if path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			var f File
			if err := json.Unmarshal(raw, &f); err != nil {
				return nil, fmt.Errorf("config %s: %w", path, err)
			}
			for _, fp := range f.Providers {
				reg.Add(fileProvider(fp))
			}
			if f.DefaultProvider != "" {
				reg.SetDefault(f.DefaultProvider)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
	}

	if v := os.Getenv("KEEL_PROVIDER"); v != "" {
		reg.SetDefault(v)
	}
	return reg, nil
}

func presetProvider(p Preset, override *FileProvider) provider.Provider {
	key := os.Getenv(p.APIKeyEnv)
	headers := map[string]string{}
	if override != nil {
		if override.APIKey != "" {
			key = expand(override.APIKey)
		}
		if override.APIKeyEnv != "" {
			key = os.Getenv(override.APIKeyEnv)
		}
		for k, v := range override.Headers {
			headers[k] = v
		}
	}
	return provider.NewOpenAICompatible(provider.OpenAICompatibleOptions{
		Name:          p.Name,
		BaseURL:       p.BaseURL,
		APIKey:        key,
		Headers:       headers,
		SessionHeader: p.SessionHeader,
		DefaultModel:  p.DefaultModel,
		Models:        p.Models,
		NoProxy:       p.NoProxy,
	})
}

func fileProvider(fp FileProvider) provider.Provider {
	key := expand(fp.APIKey)
	if key == "" && fp.APIKeyEnv != "" {
		key = os.Getenv(fp.APIKeyEnv)
	}
	return provider.NewOpenAICompatible(provider.OpenAICompatibleOptions{
		Name:          fp.Name,
		BaseURL:       fp.BaseURL,
		APIKey:        key,
		Headers:       fp.Headers,
		SessionHeader: fp.SessionHeader,
		DefaultModel:  fp.DefaultModel,
		Models:        fp.Models,
		NoProxy:       fp.NoProxy,
	})
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expand resolves ${VAR} references so a config file can point at the
// environment instead of embedding a secret.
func expand(s string) string {
	return envRef.ReplaceAllStringFunc(s, func(m string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(m, "${"), "}")
		return os.Getenv(name)
	})
}
