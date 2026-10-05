package model

import (
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// evgAnchorsKey is the internal YAML key Evergreen injects as a preamble when
// processing include files. It holds anchor definitions from prior files so
// the YAML parser can resolve cross-file aliases.
const evgAnchorsKey = "_evg_anchors"

// maxAnchorExpansionNodes caps the total number of nodes copied while expanding aliases in a
// single file's anchors to bound memory for configs with deeply chained anchor references.
const maxAnchorExpansionNodes = 100000

// maxAnchorPreambleBytes caps the size of the serialized anchor preamble. The
// preamble is re-parsed for every include file, so this bounds the cumulative added parsing.
const maxAnchorPreambleBytes = 1024 * 1024

// anchorEntry holds a single YAML anchor definition: its name (the &name tag)
// and the Node that carries it.
type anchorEntry struct {
	name string
	node *yaml.Node
}

// anchorRegistry accumulates YAML anchor definitions across include files for cross-file alias resolution
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
// in place, so the latest definition wins for subsequent files. If collection
// fails (expansion exceeded its node budget), the registry is left unchanged.
func (a *anchorRegistry) mergeAnchorsFrom(node *yaml.Node) error {
	if a == nil {
		return nil
	}
	anchors, err := collectAnchors(node)
	if err != nil {
		return err
	}
	for _, anchor := range anchors {
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
	return nil
}

// collectAnchors walks node in pre-order and returns all anchor definitions
// in encounter order. Each returned node is a self-contained copy with aliases
// expanded. Returns an error if the total expansion across all anchors exceeds
// maxAnchorExpansionNodes.
func collectAnchors(node *yaml.Node) ([]anchorEntry, error) {
	if node == nil {
		return nil, nil
	}
	nodeLimit := maxAnchorExpansionNodes
	var entries []anchorEntry
	var walk func(*yaml.Node) error
	walk = func(n *yaml.Node) error {
		if n == nil || n.Kind == yaml.AliasNode {
			return nil
		}
		if n.Anchor != "" {
			expanded, err := expandAliases(n, &nodeLimit)
			if err != nil {
				return errors.Wrapf(err, "expanding anchor '%s'", n.Anchor)
			}
			// expandAliases strips the anchor so restore it before storing.
			expanded.Anchor = n.Anchor
			entries = append(entries, anchorEntry{name: n.Anchor, node: expanded})
		}
		for _, child := range n.Content {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(node); err != nil {
		return nil, err
	}
	return entries, nil
}

// expandAliases returns a deep copy of node with every alias replaced by a copy
// of the node it references, and all anchor names stripped. Nested anchors are
// collected as their own registry entries, so stripping them here avoids
// duplicate definitions in the preamble. Each copied node decrements nodeLimit to
// bound memory as a fail-safe.
func expandAliases(node *yaml.Node, nodeLimit *int) (*yaml.Node, error) {
	if node == nil {
		return nil, nil
	}
	if node.Kind == yaml.AliasNode {
		return expandAliases(node.Alias, nodeLimit)
	}
	*nodeLimit--
	if *nodeLimit < 0 {
		return nil, errors.Errorf("expansion exceeded the maximum of %d nodes per file", maxAnchorExpansionNodes)
	}
	copied := *node
	copied.Anchor = ""
	if len(node.Content) > 0 {
		copied.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			expandedChild, err := expandAliases(child, nodeLimit)
			if err != nil {
				return nil, err
			}
			copied.Content[i] = expandedChild
		}
	}
	return &copied, nil
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
	if err != nil {
		return nil, errors.Wrap(err, "building anchor preamble")
	}
	if len(out) > maxAnchorPreambleBytes {
		return nil, errors.Errorf("anchor preamble size %d exceeds the maximum of %d bytes", len(out), maxAnchorPreambleBytes)
	}
	return out, nil
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
