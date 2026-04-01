package feature

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"
)

func TestGraphAddNode(t *testing.T) {
	g := NewGraph[string]()
	g.AddNode("a", "alpha")

	if !g.HasNode("a") {
		t.Error("expected node 'a' to exist")
	}
	if g.HasNode("b") {
		t.Error("node 'b' should not exist")
	}
}

func TestGraphAddNodeDuplicate(t *testing.T) {
	g := NewGraph[string]()
	g.AddNode("a", "alpha")
	g.AddNode("a", "alpha-updated")

	if !g.HasNode("a") {
		t.Error("expected node 'a' to exist")
	}
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "alpha-updated" {
		t.Errorf("got %v, want [alpha-updated]", result)
	}
}

func TestGraphAddEdge(t *testing.T) {
	g := NewGraph[string]()
	g.AddNode("a", "alpha")
	g.AddNode("b", "bravo")

	if err := g.AddEdge("a", "b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !g.edges["a"]["b"] {
		t.Error("expected edge a -> b")
	}
	if g.edges["b"]["a"] {
		t.Error("edge b -> a should not exist")
	}
}

func TestGraphAddEdgeDuplicate(t *testing.T) {
	g := NewGraph[string]()
	g.AddNode("a", "alpha")
	g.AddNode("b", "bravo")

	if err := g.AddEdge("a", "b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Adding the same edge again should not error or change in-degree.
	if err := g.AddEdge("a", "b"); err != nil {
		t.Fatalf("unexpected error on duplicate edge: %v", err)
	}

	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d results, want 2", len(result))
	}
}

func TestGraphAddEdgeMissingNode(t *testing.T) {
	g := NewGraph[string]()
	g.AddNode("a", "alpha")

	if err := g.AddEdge("a", "b"); err == nil {
		t.Error("expected error for missing target node")
	}
	if err := g.AddEdge("b", "a"); err == nil {
		t.Error("expected error for missing source node")
	}
}

func TestGraphAddEdgeSelf(t *testing.T) {
	g := NewGraph[string]()
	g.AddNode("a", "alpha")

	if err := g.AddEdge("a", "a"); err == nil {
		t.Error("expected error for self-edge")
	}
}

func TestGraphSortLinear(t *testing.T) {
	g := NewGraph[string]()
	g.AddNode("c", "charlie")
	g.AddNode("a", "alpha")
	g.AddNode("b", "bravo")

	// a -> b -> c
	if err := g.AddEdge("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdge("b", "c"); err != nil {
		t.Fatal(err)
	}

	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if len(result) != len(want) {
		t.Fatalf("got %d results, want %d", len(result), len(want))
	}
	for i, v := range want {
		if result[i] != v {
			t.Errorf("result[%d] = %q, want %q", i, result[i], v)
		}
	}
}

func TestGraphSortDiamond(t *testing.T) {
	g := NewGraph[string]()
	g.AddNode("a", "alpha")
	g.AddNode("b", "bravo")
	g.AddNode("c", "charlie")
	g.AddNode("d", "delta")

	// a -> b -> d
	// a -> c -> d
	if err := g.AddEdge("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdge("a", "c"); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdge("b", "d"); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdge("c", "d"); err != nil {
		t.Fatal(err)
	}

	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 4 {
		t.Fatalf("got %d results, want 4", len(result))
	}
	// a must be first, d must be last.
	if result[0] != "alpha" {
		t.Errorf("result[0] = %q, want %q", result[0], "alpha")
	}
	if result[3] != "delta" {
		t.Errorf("result[3] = %q, want %q", result[3], "delta")
	}
	// b and c can be in any order, but should be deterministic (bravo < charlie).
	if result[1] != "bravo" || result[2] != "charlie" {
		t.Errorf("result[1:3] = %v, want [bravo, charlie]", result[1:3])
	}
}

func TestGraphSortIndependent(t *testing.T) {
	g := NewGraph[string]()
	g.AddNode("c", "charlie")
	g.AddNode("a", "alpha")
	g.AddNode("b", "bravo")

	// No edges: sorted alphabetically by key.
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if len(result) != len(want) {
		t.Fatalf("got %d results, want %d", len(result), len(want))
	}
	for i, v := range want {
		if result[i] != v {
			t.Errorf("result[%d] = %q, want %q", i, result[i], v)
		}
	}
}

func TestGraphSortCircular(t *testing.T) {
	g := NewGraph[string]()
	g.AddNode("a", "alpha")
	g.AddNode("b", "bravo")

	if err := g.AddEdge("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdge("b", "a"); err != nil {
		t.Fatal(err)
	}

	_, err := g.Sort()
	if err == nil {
		t.Error("expected error for circular dependency")
	}
}

func TestGraphSortEmpty(t *testing.T) {
	g := NewGraph[string]()
	result, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty graph, got %v", result)
	}
}

func TestGraphSort_PropertyInvariant(t *testing.T) {
	// Fixed seed for reproducibility across runs.
	rng := rand.New(rand.NewSource(42))

	for trial := range 200 {
		n := rng.Intn(10) + 3 // 3..12 nodes
		keys := make([]string, n)
		for i := range n {
			keys[i] = fmt.Sprintf("node%02d", i)
		}

		g := NewGraph[string]()
		for _, k := range keys {
			g.AddNode(k, k+"-val")
		}

		// Build a random acyclic graph by construction: shuffle node order,
		// then only add edges from earlier -> later in that order. This
		// guarantees no cycles without needing Sort() as an oracle.
		// Track edges locally to avoid coupling to g.edges internals.
		type edge struct{ from, to string }
		var edges []edge
		order := slices.Clone(keys)
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		for i := range order {
			for j := i + 1; j < len(order); j++ {
				if rng.Intn(3) != 0 { // ~33% chance of each edge
					continue
				}
				if err := g.AddEdge(order[i], order[j]); err == nil {
					edges = append(edges, edge{order[i], order[j]})
				}
			}
		}

		result, err := g.Sort()
		if err != nil {
			t.Fatalf("trial %d: unexpected cycle error: %v", trial, err)
		}

		// Invariant 1: all nodes present exactly once (no duplicates, no missing).
		if len(result) != n {
			t.Fatalf("trial %d: got %d nodes, want %d", trial, len(result), n)
		}
		seen := make(map[string]struct{}, n)
		for _, v := range result {
			if _, dup := seen[v]; dup {
				t.Fatalf("trial %d: duplicate node value in result: %q", trial, v)
			}
			seen[v] = struct{}{}
		}
		for _, k := range keys {
			if _, ok := seen[k+"-val"]; !ok {
				t.Fatalf("trial %d: missing node value in result: %q", trial, k+"-val")
			}
		}

		// Invariant 2: topological order — for every edge (from→to), from appears before to.
		indexOf := make(map[string]int, n)
		for i, v := range result {
			indexOf[v] = i
		}
		for _, e := range edges {
			fromVal := e.from + "-val"
			toVal := e.to + "-val"
			if indexOf[fromVal] >= indexOf[toVal] {
				t.Errorf("trial %d: edge %s->%s violated: %s at %d, %s at %d",
					trial, e.from, e.to, fromVal, indexOf[fromVal], toVal, indexOf[toVal])
			}
		}

		// Invariant 3: deterministic — same graph sorts identically.
		result2, err := g.Sort()
		if err != nil {
			t.Fatalf("trial %d: second sort error: %v", trial, err)
		}
		if !slices.Equal(result, result2) {
			t.Errorf("trial %d: sort is not deterministic\n  first:  %v\n  second: %v", trial, result, result2)
		}
	}
}

func TestGraphSortDeterministic(t *testing.T) {
	// Run multiple times to verify determinism.
	for i := range 10 {
		g := NewGraph[string]()
		g.AddNode("z", "zulu")
		g.AddNode("m", "mike")
		g.AddNode("a", "alpha")
		g.AddNode("f", "foxtrot")

		result, err := g.Sort()
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		want := []string{"alpha", "foxtrot", "mike", "zulu"}
		for j, v := range want {
			if result[j] != v {
				t.Errorf("iteration %d: result[%d] = %q, want %q", i, j, result[j], v)
			}
		}
	}
}
