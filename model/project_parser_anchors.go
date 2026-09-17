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
// cross-file alias resolution. Entries are self-contained: aliases are expanded
// at collection time, so no entry references another and preamble order is
// irrelevant. See docs/decisions/2026-09-16_cross_file_yaml_anchors.md.
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
// in place, so the latest definition wins for subsequent files.
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

// collectAnchors walks node in pre-order and returns all anchor definitions
// (&name) in encounter order. Each returned node is a self-contained copy with
// aliases expanded, so its value is frozen at collection time: redefining an
// anchor later never changes the value of other anchors that referenced it.
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
			expanded := expandAliases(n)
			// Restore the anchor name on the top-level node so it defines
			// &name in the preamble; expandAliases strips all anchors.
			expanded.Anchor = n.Anchor
			entries = append(entries, anchorEntry{name: n.Anchor, node: expanded})
		}
		for _, child := range n.Content {
			walk(child)
		}
	}
	walk(node)
	return entries
}

// expandAliases returns a deep copy of node with every alias replaced by a copy
// of the node it references, and all anchor names stripped. Nested anchors are
// collected as their own registry entries, so stripping them here avoids
// duplicate definitions in the preamble.
func expandAliases(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode {
		return expandAliases(node.Alias)
	}
	copied := *node
	copied.Anchor = ""
	if len(node.Content) > 0 {
		copied.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			copied.Content[i] = expandAliases(child)
		}
	}
	return &copied
}

// buildAnchorPreamble marshals all registry entries into a YAML document under
// the _evg_anchors key. Prepending the returned bytes to an include file's raw
// bytes before parsing makes all accumulated anchor definitions visible to the
// YAML parser, enabling cross-file alias resolution.
func buildAnchorPreamble(registry *anchorRegistry) ([]byte, error) {
	if registry.Length() == 0 {
		return nil, nil
	}
	seqContent := make([]*yaml.Node, 0, len(registry.entries))
	for _, e := range registry.entries {
		seqContent = append(seqContent, e.node)
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
