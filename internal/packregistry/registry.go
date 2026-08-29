package packregistry

import (
	"sort"
	"sync"

	"github.com/causalens/causalens/internal/contracts"
)

// Registry is a pack-agnostic registry of System Pack implementations keyed by
// a short implementation token ("img" or environment value). Core and the
// replay worker resolve a pack by the token the deployment selects (PACK_IMPL),
// never by hard-coding a concrete pack package. A registration is a tiny wiring
// action; the Core API and replay worker themselves contain no scenario logic.
type Registry struct {
	mu    sync.RWMutex
	packs map[string]func() contracts.SystemPack
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{packs: map[string]func() contracts.SystemPack{}}
}

// defaultRegistry is the process-wide pack-selection registry shared by the
// Core API and the replay worker. It is populated here with the honestly-named
// dev pack so both binaries can exercise the replay path without Member 1's
// checkout pack; the real pack is registered via the same Resolve/Register path
// at wiring time without core logic knowing about it.
var defaultRegistry = New()

func init() {
	defaultRegistry.Register(DevImplementation, func() contracts.SystemPack { return NewDevPack() })
}

// Resolve returns the deployment's System Pack for the PACK_IMPL environment
// value, or nil when the value is empty or names no registered implementation.
func Resolve(env string) contracts.SystemPack {
	return defaultRegistry.Resolve(env)
}

// RegisterDefault binds an implementation token to a factory in the shared
// default registry. Wiring files (a single owned path per binary) use this to
// add a real pack without editing core logic.
func RegisterDefault(name string, factory func() contracts.SystemPack) {
	defaultRegistry.Register(name, factory)
}

// Register binds an implementation token to a factory. A nil name or factory is
// ignored so wiring files can register unconditionally. The last registration
// wins, which mirrors the top the integration branch takes.
func (r *Registry) Register(name string, factory func() contracts.SystemPack) {
	if name == "" || factory == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.packs[name] = factory
}

// Resolve returns a constructed pack for the token, or nil when the token is
// empty or unknown. A token that resolves to a factory returning nil is treated
// as unresolved so callers observe the same "no pack" path as an empty token.
func (r *Registry) Resolve(name string) contracts.SystemPack {
	r.mu.RLock()
	factory, ok := r.packs[name]
	r.mu.RUnlock()
	if !ok || factory == nil {
		return nil
	}
	return factory()
}

// Names returns the sorted list of registered implementation tokens.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.packs))
	for name := range r.packs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
