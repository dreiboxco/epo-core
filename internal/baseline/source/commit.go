// Package source defines the commit record shared by every baseline analyzer.
//
// Keeping this type in its own package lets analyzers (busfactor, hotspots,
// future ones) depend on a small, stable domain model without taking each
// other as transitive dependencies.
package source

import "time"

// Commit is the minimal slice of a VCS commit consumed by the analyzers.
// Loaders (e.g. internal/baseline/git) populate the fields they have; missing
// values are zero-valued. An analyzer that doesn't need Message can simply
// ignore it.
type Commit struct {
	// SHA is the commit identifier. Used only for diagnostics.
	SHA string
	// Author identifies who made the commit. Email is the typical key, but
	// callers may pass a normalized identity (e.g. after .mailmap merging).
	Author string
	// When is the commit timestamp. Used for windowing.
	When time.Time
	// Files lists the files touched by the commit.
	Files []string
	// Message is the commit message (subject + body). Required by analyzers
	// that classify commits, e.g. hotspots needs this to detect bug fixes.
	Message string
}
