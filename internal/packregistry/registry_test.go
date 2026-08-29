package packregistry

import (
	"testing"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/differential"
)

// Compile-time interface assertions: the dev pack must satisfy the frozen
// SystemPack interface and the structural alignment contract used by the
// differential analyzer.
var (
	_ contracts.SystemPack   = (*DevPack)(nil)
	_ differential.Alignment = (*DevPack)(nil)
)

func TestDefaultRegistryHealthy(t *testing.T) {
	r := New()
	if got := r.Names(); len(got) != 0 {
		t.Fatalf("fresh registry has names: %v", got)
	}
	if pack := r.Resolve(DevImplementation); pack != nil {
		t.Fatalf("fresh registry resolved dev pack without registration")
	}
}

func TestRegisterAndResolve(t *testing.T) {
	r := New()
	r.Register(DevImplementation, func() contracts.SystemPack { return NewDevPack() })

	pack := r.Resolve(DevImplementation)
	if pack == nil {
		t.Fatal("dev pack not resolved")
	}
	if got := pack.Descriptor().ID; got != "checkout_duplicate_effect_dev" {
		t.Fatalf("descriptor id = %q", got)
	}
	if got := pack.Descriptor().Version; got != "0.0.0-dev" {
		t.Fatalf("descriptor version = %q", got)
	}
	if r.Resolve("unknown") != nil {
		t.Fatal("unknown token should not resolve")
	}
	if r.Resolve("") != nil {
		t.Fatal("empty token should not resolve")
	}
}

func TestResolveUsesLatestRegistration(t *testing.T) {
	r := New()
	r.Register("t", func() contracts.SystemPack { return nil })
	if pack := r.Resolve("t"); pack != nil {
		t.Fatalf("nil factory should not resolve to a pack")
	}
	r.Register("t", func() contracts.SystemPack { return NewDevPack() })
	if pack := r.Resolve("t"); pack == nil {
		t.Fatal("later registration should win")
	}
}

func TestRegisterIgnoresInvalid(t *testing.T) {
	r := New()
	r.Register("", func() contracts.SystemPack { return NewDevPack() })
	r.Register("x", nil)
	if r.Resolve("") != nil || r.Resolve("x") != nil {
		t.Fatal("invalid registrations should be ignored")
	}
}

func TestNamesSorted(t *testing.T) {
	r := New()
	r.Register("zeta", func() contracts.SystemPack { return NewDevPack() })
	r.Register("alpha", func() contracts.SystemPack { return NewDevPack() })
	names := r.Names()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "zeta" {
		t.Fatalf("names unsorted: %v", names)
	}
}
