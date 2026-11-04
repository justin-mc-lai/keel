package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings are the user-editable knobs the settings panel writes. They live in
// the same JSON file as providers, which is never inside the repository.
type Settings struct {
	DefaultProvider string `json:"defaultProvider,omitempty"`
	DefaultApproval string `json:"defaultApproval,omitempty"`
	WorkspaceRoot   string `json:"workspaceRoot,omitempty"`
}

// LoadSettings reads the settings block, returning zero values when the file
// does not exist.
func LoadSettings() Settings {
	path := Path()
	if path == "" {
		return Settings{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Settings{}
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return Settings{}
	}
	return Settings{
		DefaultProvider: f.DefaultProvider,
		DefaultApproval: f.DefaultApproval,
		WorkspaceRoot:   f.WorkspaceRoot,
	}
}

// SaveSettings merges the settings into the existing config file so provider
// entries and any unknown keys survive.
func SaveSettings(s Settings) error {
	path := Path()
	if path == "" {
		return os.ErrNotExist
	}
	doc := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &doc)
	}
	if s.DefaultProvider != "" {
		doc["defaultProvider"] = s.DefaultProvider
	}
	if s.DefaultApproval != "" {
		doc["defaultApproval"] = s.DefaultApproval
	}
	if s.WorkspaceRoot != "" {
		doc["workspaceRoot"] = s.WorkspaceRoot
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}
