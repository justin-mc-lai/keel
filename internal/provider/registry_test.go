package provider_test

import (
	"testing"

	"github.com/justin-mc-lai/keel/internal/config"
)

func TestRegistryListsProviders(t *testing.T) {
	reg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	infos := reg.List()
	if len(infos) < 2 {
		t.Fatalf("want 2+ providers, got %d", len(infos))
	}
	byName := map[string]bool{}
	for _, info := range infos {
		byName[info.Name] = true
		if info.DefaultModel == "" {
			t.Errorf("%s: empty default model", info.Name)
		}
		if len(info.Models) == 0 {
			t.Errorf("%s: empty model list", info.Name)
		}
	}
	for _, want := range []string{"opencode-go", "commandcode"} {
		if !byName[want] {
			t.Errorf("missing provider %q (have %v)", want, reg.Names())
		}
	}
}
