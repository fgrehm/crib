package feature

import (
	"fmt"
	"sort"
)

// Graph is a generic directed acyclic graph that supports topological sorting
// via Kahn's algorithm. Node keys are strings, values are of type T.
type Graph[T any] struct {
	nodes    map[string]T
	edges    map[string]map[string]bool
	inDegree map[string]int
}

// NewGraph creates an empty graph.
func NewGraph[T any]() *Graph[T] {
	return &Graph[T]{
		nodes:    make(map[string]T),
		edges:    make(map[string]map[string]bool),
		inDegree: make(map[string]int),
	}
}

// AddNode adds a node to the graph. If the node already exists, its value
// is updated.
func (g *Graph[T]) AddNode(key string, value T) {
	g.nodes[key] = value
	if _, ok := g.inDegree[key]; !ok {
		g.inDegree[key] = 0
	}
}

// AddEdge adds a directed edge from -> to, meaning "from" must come before
// "to" in the sorted output. Both nodes must already exist in the graph.
func (g *Graph[T]) AddEdge(from, to string) error {
	if !g.HasNode(from) {
		return fmt.Errorf("node %q not found", from)
	}
	if !g.HasNode(to) {
		return fmt.Errorf("node %q not found", to)
	}
	if from == to {
		return fmt.Errorf("self-edge not allowed: %q", from)
	}

	if g.edges[from] == nil {
		g.edges[from] = make(map[string]bool)
	}
	if !g.edges[from][to] {
		g.edges[from][to] = true
		g.inDegree[to]++
	}
	return nil
}

// HasNode returns true if the graph contains a node with the given key.
func (g *Graph[T]) HasNode(key string) bool {
	_, ok := g.nodes[key]
	return ok
}

// HasEdge returns true if there is a directed edge from -> to.
func (g *Graph[T]) HasEdge(from, to string) bool {
	if g.edges[from] == nil {
		return false
	}
	return g.edges[from][to]
}

// Sort returns nodes in topological order using Kahn's algorithm.
// When multiple nodes have zero in-degree, they are processed in sorted
// key order for deterministic output. Returns an error if the graph
// contains a cycle.
func (g *Graph[T]) Sort() ([]T, error) {
	if len(g.nodes) == 0 {
		return nil, nil
	}

	// Copy in-degree map so we don't mutate the graph.
	inDegree := make(map[string]int, len(g.inDegree))
	for k, v := range g.inDegree {
		inDegree[k] = v
	}

	// Collect initial zero-degree nodes, sorted for determinism.
	var queue []string
	for key := range g.nodes {
		if inDegree[key] == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)

	var result []T
	for len(queue) > 0 {
		// Pop front.
		key := queue[0]
		queue = queue[1:]
		result = append(result, g.nodes[key])

		// Collect neighbors whose in-degree drops to zero.
		var newZero []string
		for neighbor := range g.edges[key] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				newZero = append(newZero, neighbor)
			}
		}

		// Sort new zero-degree nodes and insert into queue in sorted position.
		sort.Strings(newZero)
		queue = sortedMerge(queue, newZero)
	}

	if len(result) != len(g.nodes) {
		return nil, fmt.Errorf("circular dependency detected")
	}

	return result, nil
}

// sortedMerge merges two sorted slices into a single sorted slice.
func sortedMerge(a, b []string) []string {
	result := make([]string, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] <= b[j] {
			result = append(result, a[i])
			i++
		} else {
			result = append(result, b[j])
			j++
		}
	}
	result = append(result, a[i:]...)
	result = append(result, b[j:]...)
	return result
}
