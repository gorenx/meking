// Package epoch owns the unified query publication version. It decides
// when unpublished Knowledge changes require catch-up and atomically selects a
// complete set of immutable cross-context identities after readiness checks.
package epoch
