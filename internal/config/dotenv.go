package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv reads keys from local dotenv files. Existing environment
// variables always win, so a shell export is never overridden.
//
// Order: ./.env.local, ./.env, then ~/.agentdesk/.env. The last one exists
// because a GUI launch has no useful working directory and inherits no shell
// exports — without it the desktop app can never authenticate.
func LoadDotEnv() {
	for _, path := range dotEnvPaths() {
		readDotEnv(path)
	}
}

func dotEnvPaths() []string {
	paths := []string{".env.local", ".env"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".agentdesk", ".env"))
	}
	return paths
}

func readDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}
