package kube_test

import (
	"testing"

	"github.com/mo-pankaj/kctl/internal/kube"
)

func TestRiskFor(t *testing.T) {
	rules := []kube.RiskRule{
		{Server: "https://api.prod.example.com", Level: kube.RiskProtected},
		{Server: "https://apiserver-live-*.example.net", Level: kube.RiskProtected},
	}

	tests := []struct {
		name   string
		server string
		want   kube.RiskLevel
	}{
		{name: "exact match is protected", server: "https://api.prod.example.com", want: kube.RiskProtected},
		{name: "wildcard match is protected", server: "https://apiserver-live-9f2c.example.net", want: kube.RiskProtected},
		{name: "unlisted cluster fails open to normal", server: "https://127.0.0.1:6443", want: kube.RiskNormal},
		{name: "near miss does not match", server: "https://api.prod.example.com.evil.test", want: kube.RiskNormal},
		{name: "empty server is normal", server: "", want: kube.RiskNormal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := kube.RiskFor(tt.server, rules)

			if got != tt.want {
				t.Fatalf("RiskFor(%q) = %q, want %q", tt.server, got, tt.want)
			}
		})
	}
}

func TestRiskIsNotKeyedOnContextName(t *testing.T) {
	// The whole point: a context nicknamed "dev" pointing at the production
	// API server must still be protected.
	rules := []kube.RiskRule{{Server: "https://api.prod.example.com", Level: kube.RiskProtected}}

	got := kube.RiskFor("https://api.prod.example.com", rules)

	if got != kube.RiskProtected {
		t.Fatalf("a prod server reached through a friendly context name resolved to %q, want protected", got)
	}
}

func TestNoRulesMeansEverythingIsNormal(t *testing.T) {
	got := kube.RiskFor("https://anything", nil)

	if got != kube.RiskNormal {
		t.Fatalf("RiskFor with no rules = %q, want normal", got)
	}
}
