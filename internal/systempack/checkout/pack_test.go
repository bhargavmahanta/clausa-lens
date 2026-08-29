package checkout

import "testing"

func TestPack_Descriptor(t *testing.T) {
	p := New()
	d := p.Descriptor()
	if d.ID != "checkout_duplicate_effect" {
		t.Fatalf("ID = %q, want checkout_duplicate_effect", d.ID)
	}
	if d.Version != "1.0.0" {
		t.Fatalf("Version = %q, want 1.0.0", d.Version)
	}
	if d.InterfaceVersion != "1.0" {
		t.Fatalf("InterfaceVersion = %q, want 1.0", d.InterfaceVersion)
	}
}

func TestPack_Labels(t *testing.T) {
	p := New()
	labels := p.Labels()

	for _, component := range []string{"gateway", "checkout", "payment", "ledger"} {
		if labels.Components[component] == "" {
			t.Fatalf("missing label for component %q", component)
		}
	}
	if labels.EventTypes["TIMEOUT"] == "" {
		t.Fatalf("missing label for event_type TIMEOUT")
	}
	if labels.Interventions["PAYMENT_LATENCY"] == "" {
		t.Fatalf("missing label for intervention PAYMENT_LATENCY")
	}
}
