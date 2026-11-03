package provider

import "sort"

// Info is the serializable view of a provider used by the CLI and the desktop
// bindings.
type Info struct {
	Name         string   `json:"name"`
	DefaultModel string   `json:"defaultModel"`
	Models       []string `json:"models"`
	Available    bool     `json:"available"`
}

// Registry holds the configured providers. It is the single place that knows
// which vendors exist, so nothing else has to.
type Registry struct {
	order       []string
	byName      map[string]Provider
	defaultName string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byName: map[string]Provider{}}
}

// Add registers a provider. A later provider with the same name replaces the
// earlier one (that is how a user config overrides a built-in preset).
func (r *Registry) Add(p Provider) {
	if _, exists := r.byName[p.Name()]; !exists {
		r.order = append(r.order, p.Name())
	}
	r.byName[p.Name()] = p
	if r.defaultName == "" {
		r.defaultName = p.Name()
	}
}

// SetDefault sets the provider used when the caller does not specify one.
func (r *Registry) SetDefault(name string) {
	if _, ok := r.byName[name]; ok {
		r.defaultName = name
	}
}

// Get resolves a provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.byName[name]
	return p, ok
}

// Default returns the default provider, or nil when the registry is empty.
func (r *Registry) Default() Provider {
	return r.byName[r.defaultName]
}

// DefaultName returns the configured default provider name.
func (r *Registry) DefaultName() string { return r.defaultName }

// Names returns registered provider names in insertion order.
func (r *Registry) Names() []string {
	return append([]string(nil), r.order...)
}

// List returns provider info, sorted by name for stable CLI output.
func (r *Registry) List() []Info {
	out := make([]Info, 0, len(r.order))
	for _, name := range r.order {
		p := r.byName[name]
		out = append(out, Info{
			Name:         p.Name(),
			DefaultModel: p.DefaultModel(),
			Models:       p.Models(),
			Available:    p.Name() != "" && hasKey(p),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func hasKey(p Provider) bool {
	// Only the built-in transport declares its credential state. A foreign
	// implementation is reported as unavailable rather than assumed ready.
	if o, ok := p.(*OpenAICompatible); ok {
		return o.apiKey != ""
	}
	return false
}
