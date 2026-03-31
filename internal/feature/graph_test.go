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

		// Add random edges, skipping any that would create a cycle.
		for i := range n {
			for j := range n {
				if i == j {
					continue
				}
				if rng.Intn(3) != 0 { // ~33% chance of each edge
					continue
				}
				// Test if edge creates a cycle by attempting a sort first.
				if err := g.AddEdge(keys[i], keys[j]); err != nil {
					continue // self-edge or missing node (shouldn't happen)
				}
				if _, err := g.Sort(); err != nil {
					// Cycle introduced: remove by rebuilding without this edge.
					g2 := NewGraph[string]()
					for _, k := range keys {
						g2.AddNode(k, k+"-val")
					}
					for from, tos := range g.edges {
						for to := range tos {
							if from == keys[i] && to == keys[j] {
								continue
							}
							_ = g2.AddEdge(from, to) //nolint:errcheck
						}
					}
					g = g2
				}
			}
		}

		result, err := g.Sort()
		if err != nil {
			t.Fatalf("trial %d: unexpected cycle error: %v", trial, err)
		}

		// Invariant 1: all nodes present.
		if len(result) != n {
			t.Fatalf("trial %d: got %d nodes, want %d", trial, len(result), n)
		}

		// Invariant 2: topological order — for every edge (from→to), from appears before to.
		indexOf := make(map[string]int, n)
		for i, v := range result {
			indexOf[v] = i
		}
		for from, tos := range g.edges {
			for to := range tos {
				fromVal := from + "-val"
				toVal := to + "-val"
				if indexOf[fromVal] >= indexOf[toVal] {
					t.Errorf("trial %d: edge %s->%s violated: %s at %d, %s at %d",
						trial, from, to, fromVal, indexOf[fromVal], toVal, indexOf[toVal])
				}
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
