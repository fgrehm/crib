package feature

import (
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
	if !g.HasEdge("a", "b") {
		t.Error("expected edge a -> b")
	}
	if g.HasEdge("b", "a") {
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

func TestGraphSortDeterministic(t *testing.T) {
	// Run multiple times to verify determinism.
	for i := 0; i < 10; i++ {
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
