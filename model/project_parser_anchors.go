package model

import (
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// evgAnchorsKey is the internal YAML key Evergreen injects as a preamble when
// processing include files. It holds anchor definitions from prior files so
// the YAML parser can resolve cross-file aliases.
const evgAnchorsKey = "_evg_anchors"

// anchorEntry holds a single YAML anchor definition: its name (the &name tag)
// and the Node that carries it.
type anchorEntry struct {
	name string
	node *yaml.Node
}

// anchorRegistry accumulates YAML anchor definitions across include files for
// cross-file alias resolution.
//
// Invariant: for any entry whose content contains a *alias to another entry,
// the aliased entry must appear earlier in the slice. buildAnchorPreamble
// serializes the registry to YAML text, and the YAML spec requires &name to be
// defined before any *name that references it.
type anchorRegistry struct {
	entries []anchorEntry
}

// Length returns the number of entries, or 0 if the receiver is nil.
func (a *anchorRegistry) Length() int {
	if a == nil {
		return 0
	}
	return len(a.entries)
}

// mergeAnchorsFrom collects all anchor definitions from node and merges them
// into the registry. New anchors are appended; redefined anchors are updated
// in place (not moved to the end).
//
// Updating in place is critical for the invariant: if anchor Q is in the
// registry at position P and references *X, and a later file redefines &X,
// moving X to the end (past P) would place the definition after Q's reference —
// a forward reference. Keeping X at its original position preserves the
// ordering that Q depends on. Any remaining violations are resolved by
// buildAnchorPreamble via topological sort.
func (a *anchorRegistry) mergeAnchorsFrom(node *yaml.Node) {
	if a == nil {
		return
	}
	for _, anchor := range collectAnchors(node) {
		found := false
		for i, existing := range a.entries {
			if existing.name == anchor.name {
				a.entries[i] = anchor
				found = true
				break
			}
		}
		if !found {
			a.entries = append(a.entries, anchor)
		}
	}
}

// collectAnchors walks node in pre-order and returns all anchored nodes in
// encounter order. AliasNodes are not followed, so only anchor definitions
// (&name) are collected, never alias uses (*name).
func collectAnchors(node *yaml.Node) []anchorEntry {
	if node == nil {
		return nil
	}
	var entries []anchorEntry
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil || n.Kind == yaml.AliasNode {
			return
		}
		if n.Anchor != "" {
			entries = append(entries, anchorEntry{name: n.Anchor, node: n})
		}
		for _, child := range n.Content {
			walk(child)
		}
	}
	walk(node)
	return entries
}

// buildAnchorPreamble marshals all registry entries into a YAML document under
// the _evg_anchors key. Prepending the returned bytes to an include file's raw
// bytes before parsing makes all accumulated anchor definitions visible to the
// YAML parser, enabling cross-file alias resolution.
//
// Entries are emitted in topological dependency order: if anchor A's content
// contains an alias to anchor B, B is emitted before A. This guarantees the
// preamble is valid YAML regardless of the order anchors were collected.
func buildAnchorPreamble(registry *anchorRegistry) ([]byte, error) {
	if registry.Length() == 0 {
		return nil, nil
	}
	sorted := preambleOrder(registry)
	seqContent := make([]*yaml.Node, 0, len(sorted))
	for _, idx := range sorted {
		seqContent = append(seqContent, registry.entries[idx].node)
	}
	preambleDoc := &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{
			{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: evgAnchorsKey, Tag: "!!str"},
					{Kind: yaml.SequenceNode, Content: seqContent},
				},
			},
		},
	}
	out, err := yaml.Marshal(preambleDoc)
	return out, errors.Wrap(err, "building anchor preamble")
}

// preambleOrder returns registry indices in topological order using a
// post-order DFS: each entry is emitted only after all entries it depends on
// (via alias nodes). This guarantees &name appears before any *name that
// references it in the serialized preamble.
//
// Cycles are not expected in valid YAML. If one exists, visited nodes are
// skipped and the preamble will fail to parse; the caller falls back to
// parsing without the preamble.
func preambleOrder(registry *anchorRegistry) []int {
	n := len(registry.entries)
	nameToIdx := make(map[string]int, n)
	for i, e := range registry.entries {
		nameToIdx[e.name] = i
	}
	visited := make([]bool, n)
	order := make([]int, 0, n)
	var visit func(int)
	visit = func(i int) {
		if visited[i] {
			return
		}
		visited[i] = true
		for _, dep := range aliasDeps(registry.entries[i].node, nameToIdx) {
			visit(dep)
		}
		order = append(order, i)
	}
	for i := range registry.entries {
		visit(i)
	}
	return order
}

// aliasDeps returns the registry indices of all anchors directly referenced by
// alias nodes within node's subtree.
func aliasDeps(node *yaml.Node, nameToIdx map[string]int) []int {
	var deps []int
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		if n.Kind == yaml.AliasNode {
			if n.Alias != nil {
				if idx, ok := nameToIdx[n.Alias.Anchor]; ok {
					deps = append(deps, idx)
				}
			}
			return
		}
		for _, child := range n.Content {
			walk(child)
		}
	}
	walk(node)
	return deps
}

// stripEvgAnchorsKey removes the _evg_anchors key and its value from the
// top-level mapping in node. Returns true if the key was found and removed.
// No-op (returns false) when the key is absent.
func stripEvgAnchorsKey(node *yaml.Node) bool {
	mapping := node
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		mapping = node.Content[0]
	}
	if mapping.Kind != yaml.MappingNode {
		return false
	}
	// MappingNode.Content is key-value pairs: [k0, v0, k1, v1, ...]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == evgAnchorsKey {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return true
		}
	}
	return false
}
