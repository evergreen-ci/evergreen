# 2026-05-21 Cross-file YAML anchor support in include files

- status: approved
- date: 2026-05-21
- authors: Annie Black

## Context and Problem Statement

Evergreen project configs can split their configuration across multiple files using `include`. A common
pattern is defining reusable YAML anchors (`&name`) in one file and referencing them as aliases
(`*name`) in another. Standard YAML parsers resolve anchors only within a single document, so
cross-file aliases would previously produce an "unknown anchor" error.

## Decision Outcome

Cross-file anchor resolution is implemented as an opt-in beta feature, enabled by passing
`--yaml-anchors` to `evergreen validate`, `evergreen evaluate`, or `evergreen patch`. When not
opted in, the existing parsing path is used without modification.

### How it works

The implementation is contained in `createIntermediateProject` (`model/project_parser.go`) and the
anchor helpers in `model/project_parser_anchors.go`.

**Anchor registry.** `LoadProjectInto` initializes a `[]anchorEntry` slice (a registry of `name →
*yaml.Node` pairs) and passes a pointer to it into `createIntermediateProject` for the main config
file. The pointer is then threaded through `mergeIncludes` and passed into `createIntermediateProject`
for each include file in declaration order.

**Preamble injection.** Before parsing each file, if the registry is non-empty,
`buildAnchorPreamble` marshals the accumulated anchor nodes into a synthetic YAML document under
the key `_evg_anchors` and prepends it to the raw file bytes. When `gopkg.in/yaml.v3` parses the
combined bytes, it sees the anchor definitions first and can resolve any alias that references them.

**`_evg_anchors` key stripping.** After decoding to a `yaml.Node`, `stripEvgAnchorsKey` removes
the `_evg_anchors` mapping entry from the node tree before struct decoding and before anchor
collection. This prevents the preamble key from appearing as an unknown field and from polluting the
registry with its own artificial anchors. In strict mode, the struct used for
`UnmarshalYAMLStrictWithFallback` declares an `EvgAnchors any` field tagged `_evg_anchors` to
suppress the unknown-field error that strict unmarshalling would otherwise raise.

**Registry updates.** After parsing each file, `collectAnchors` walks the cleaned node tree and
appends any newly defined anchors to the registry. If two files define an anchor with the same name,
the later file's definition replaces the earlier one (last-write-wins), matching yaml.v3's
within-file behavior and preserving backwards compatibility for projects that redefine anchors across
files.

**yaml.v3 / yaml.v2 fallback.** `node.Decode` (yaml.v3) rejects some constructs that yaml.v2
accepts (e.g. duplicate mapping keys). When decoding into the `ParserProject` struct fails, the
implementation falls back to `UnmarshalYAMLWithFallback` (yaml.v2). The node from the first parse
is still used for anchor collection, so cross-file anchors continue to work even for files that
exercise the fallback path.

### Why `nil` registry as the feature flag

Passing a `nil` registry pointer to `createIntermediateProject` is the off switch. A nil registry
means no preamble is built, no node is decoded, and no anchors are collected — the function takes
the legacy `UnmarshalYAMLWithFallback` path with no behavioral change. This avoids any conditional
branching at call sites and keeps the legacy path identical to what shipped before this feature.
