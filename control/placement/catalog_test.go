package placement

import (
	"errors"
	"testing"
)

func TestCatalogResolveExplicitRegion(t *testing.T) {
	catalog := mustCatalog(t)

	plan, err := catalog.Resolve("US-East", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Automatic {
		t.Fatal("expected explicit plan")
	}
	if got, want := plan.Candidates, []string{"us-east"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestCatalogResolveUnknownRegion(t *testing.T) {
	catalog := mustCatalog(t)

	_, err := catalog.Resolve("mars-east", "")
	if !errors.Is(err, ErrUnknownRegion) {
		t.Fatalf("error = %v, want unknown region", err)
	}
	if got, want := err.Error(), "unknown region: mars-east"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestCatalogResolveAutomaticUsesGeoPriority(t *testing.T) {
	catalog := mustCatalog(t)

	plan, err := catalog.Resolve("", "ca")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !plan.Automatic {
		t.Fatal("expected automatic plan")
	}
	if got, want := plan.Candidates[0], "ca-central"; got != want {
		t.Fatalf("first candidate = %q, want %q", got, want)
	}
	if !contains(plan.Candidates, "us-east") {
		t.Fatalf("candidates %#v missing us-east fallback", plan.Candidates)
	}
}

func TestCatalogResolveAutomaticUsesDefault(t *testing.T) {
	catalog := mustCatalog(t)

	plan, err := catalog.Resolve("", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got, want := plan.Candidates[0], "us-east"; got != want {
		t.Fatalf("first candidate = %q, want %q", got, want)
	}
}

func TestNewCatalogRejectsUnknownDefault(t *testing.T) {
	_, err := NewCatalog(Config{
		Regions:       []string{"us-east"},
		DefaultRegion: "eu-central",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func mustCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := NewCatalog(Config{
		Regions:       []string{"us-east", "us-west", "ca-central", "eu-central"},
		DefaultRegion: "us-east",
		GeoPriority: map[string][]string{
			"CA": {"ca-central", "us-east"},
			"US": {"us-east", "us-west"},
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return catalog
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
