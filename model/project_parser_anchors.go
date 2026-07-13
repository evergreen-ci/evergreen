package model

import (
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

// anchorEntries accumulates YAML anchor definitions across include files for cross-file alias resolution.
type anchorEntries []anchorEntry

// mergeAnchorsFrom collects all anchor definitions from node and merges them into
// the registry by name, so that later include files can resolve aliases defined in
// earlier files. No-op if the receiver is nil.
//
// When an anchor name already exists, the old entry is removed and the new one is
// appended at the end. This ensures that if a redefinition introduces alias
// dependencies on anchors added by the same file, those dependencies always appear
// earlier in the slice, and therefore earlier in the marshaled preamble, satisfying
// YAML's anchor-before-alias rule.
func (a *anchorEntries) mergeAnchorsFrom(node *yaml.Node) {
	if a == nil {
		return
	}
	for _, anchor := range collectAnchors(node) {
		for i, existing := range *a {
			if existing.name == anchor.name {
				*a = append((*a)[:i], (*a)[i+1:]...)
				break
			}
		}
		*a = append(*a, anchor)
	}
}

// collectAnchors walks node in pre-order and returns all anchored nodes in
// encounter order. AliasNodes are not followed, so only anchor definitions
// (&name) are collected, never alias uses (*name).
func collectAnchors(node *yaml.Node) anchorEntries {
	if node == nil {
		return nil
	}
	var entries anchorEntries
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
// Entries must be in encounter order so that any alias references within anchor
// values (e.g. an anchor whose value itself uses an alias to an earlier anchor)
// are valid when the preamble is parsed.
func buildAnchorPreamble(entries *anchorEntries) ([]byte, error) {
	if entries == nil || len(*entries) == 0 {
		return nil, nil
	}
	seqContent := make([]*yaml.Node, 0, len(*entries))
	for _, e := range *entries {
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
	return yaml.Marshal(preambleDoc)
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
