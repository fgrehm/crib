package feature

import (
	"fmt"
	"strings"
)

// OrderFeatures sorts features respecting hard dependencies (DependsOn) and
// soft dependencies (InstallsAfter). Features listed in overrideOrder are
// moved to the front in that order, while still respecting hard dependencies.
func OrderFeatures(features []*FeatureSet, overrideOrder []string) ([]*FeatureSet, error) {
	if len(features) == 0 {
		return nil, nil
	}

	// Build two indexes: normalized ID -> config ID for dependency matching,
	// and config ID -> FeatureSet for direct lookups.
	lookup := make(map[string]string, len(features))
	byID := make(map[string]*FeatureSet, len(features))
	g := NewGraph[*FeatureSet]()
	for _, f := range features {
		lookup[normalizeID(f.ConfigID)] = f.ConfigID
		byID[f.ConfigID] = f
		g.AddNode(f.ConfigID, f)
	}

	if err := addHardDependencies(g, features, lookup); err != nil {
		return nil, err
	}
	addSoftDependencies(g, features, lookup, byID)

	sorted, err := g.Sort()
	if err != nil {
		return nil, fmt.Errorf("ordering features: %w", err)
	}

	if len(overrideOrder) > 0 {
		sorted = applyOverrideOrder(sorted, overrideOrder)
	}

	return sorted, nil
}

// addHardDependencies adds edges for DependsOn entries. Hard dependencies
// must exist in the feature set.
func addHardDependencies(g *Graph[*FeatureSet], features []*FeatureSet, lookup map[string]string) error {
	for _, f := range features {
		for depID := range f.Config.DependsOn {
			targetID, ok := lookup[normalizeID(depID)]
			if !ok {
				return fmt.Errorf("feature %q has hard dependency on %q which is not in the feature set", f.ConfigID, depID)
			}
			if err := g.AddEdge(targetID, f.ConfigID); err != nil {
				return fmt.Errorf("adding dependency %q -> %q: %w", targetID, f.ConfigID, err)
			}
		}
	}
	return nil
}

// addSoftDependencies adds edges for InstallsAfter entries. Soft dependencies
// are only added if the referenced feature exists in the set and there is no
// existing hard dependency in the reverse direction.
func addSoftDependencies(g *Graph[*FeatureSet], features []*FeatureSet, lookup map[string]string, byID map[string]*FeatureSet) {
	for _, f := range features {
		for _, afterID := range f.Config.InstallsAfter {
			targetID, ok := lookup[normalizeID(afterID)]
			if !ok {
				// Soft dependency target not in feature set, skip.
				continue
			}
			if targetID == f.ConfigID {
				continue
			}
			// Skip if there is already a hard dependency in the reverse direction
			// (target depends on f), which would create a cycle.
			if hasHardDep(byID[targetID], f.ConfigID, lookup) {
				continue
			}
			// Ignore edge errors (e.g. if it would create a duplicate).
			_ = g.AddEdge(targetID, f.ConfigID)
		}
	}
}

// hasHardDep returns true if the given feature has a hard dependency
// (DependsOn) on depID.
func hasHardDep(f *FeatureSet, depID string, lookup map[string]string) bool {
	if f == nil {
		return false
	}
	for depKey := range f.Config.DependsOn {
		if targetID, ok := lookup[normalizeID(depKey)]; ok && targetID == depID {
			return true
		}
	}
	return false
}

// applyOverrideOrder moves features matching overrideOrder IDs to the front,
// preserving their relative order. Features not in overrideOrder follow in
// their original sorted order.
func applyOverrideOrder(features []*FeatureSet, overrideOrder []string) []*FeatureSet {
	indexed := make(map[string]*FeatureSet, len(features))
	for _, f := range features {
		indexed[f.ConfigID] = f
	}

	overridden := make(map[string]bool, len(overrideOrder))
	var front []*FeatureSet
	for _, id := range overrideOrder {
		if f, ok := indexed[id]; ok {
			front = append(front, f)
			overridden[id] = true
		}
	}

	var rest []*FeatureSet
	for _, f := range features {
		if !overridden[f.ConfigID] {
			rest = append(rest, f)
		}
	}

	return append(front, rest...)
}

// normalizeID strips version tags (@digest or :tag) from OCI feature
// references. Local paths (./ or ../) and HTTP URLs are returned unchanged.
func normalizeID(id string) string {
	// Local paths: keep as-is.
	if strings.HasPrefix(id, "./") || strings.HasPrefix(id, "../") {
		return id
	}

	// HTTP(S) URLs: keep as-is.
	if strings.HasPrefix(id, "http://") || strings.HasPrefix(id, "https://") {
		return id
	}

	// Strip @digest.
	if idx := strings.Index(id, "@"); idx >= 0 {
		return id[:idx]
	}

	// Strip :tag.
	if idx := strings.LastIndex(id, ":"); idx >= 0 {
		// Make sure we don't strip the port from a registry URL like
		// localhost:5000/feature. Only strip if there is a / after
		// the last : or if : comes after the last /.
		lastSlash := strings.LastIndex(id, "/")
		if idx > lastSlash {
			return id[:idx]
		}
	}

	return id
}
