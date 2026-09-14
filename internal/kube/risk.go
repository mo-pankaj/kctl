package kube

import (
	"regexp"
	"strings"
)

// RiskLevel says how much ceremony an apply to a cluster requires.
type RiskLevel string

const (
	// RiskNormal confirms with a single keypress.
	RiskNormal RiskLevel = "normal"
	// RiskProtected requires typing the context name exactly.
	RiskProtected RiskLevel = "protected"
)

// RiskRule maps a cluster to a risk level by its API server URL.
type RiskRule struct {
	Server string
	Level  RiskLevel
}

// RiskFor resolves the risk level for a cluster's API server URL.
//
// Rules key on the SERVER URL rather than the context name on purpose. Context
// names are local nicknames: they get renamed, they are frequently shared across
// machines with different meanings, and a production cluster's context is often
// not called anything resembling "prod". The server URL is the cluster's actual
// identity.
//
// A cluster matching no rule is normal. Failing open is deliberate: the
// alternative trains the user to type a context name for every unremarkable
// apply, which is how a confirmation becomes muscle memory and stops being read.
func RiskFor(server string, rules []RiskRule) (level RiskLevel) {
	level = RiskNormal

	for _, r := range rules {
		if matchServer(r.Server, server) {
			return r.Level
		}
	}

	return level
}

// matchServer supports an exact URL or a trailing "*" wildcard.
func matchServer(pattern, server string) (ok bool) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ok
	}

	if !strings.Contains(pattern, "*") {
		ok = pattern == server

		return ok
	}

	quoted := regexp.QuoteMeta(pattern)
	quoted = "^" + strings.ReplaceAll(quoted, `\*`, ".*") + "$"

	re, err := regexp.Compile(quoted)
	if err != nil {
		return ok
	}

	ok = re.MatchString(server)

	return ok
}
