# 2026-09-16 Cross-file YAML anchors: expand aliases at collection time

* status: accepted
* date: 2026-09-16
* authors: Annie Black

## Context and Problem Statement

Evergreen supports cross-file YAML anchors: an anchor defined in one include file can be used as an alias in a later include file. This works by maintaining an `anchorRegistry` and prepending a serialized preamble of all known anchor definitions before parsing each include file.

The preamble must be valid YAML, which requires every `&name` definition to appear before any `*name` alias that references it. Two bugs came from violating this:

1. Storing anchor nodes with live alias references meant the preamble had ordering
   dependencies between entries. When an anchor was redefined by a later file, the
   registry reordered it past entries that referenced it, producing a forward
   reference. The preamble failed to parse and all cross-file anchors silently
   stopped working for subsequent files.
2. Because the preamble was re-serialized and re-parsed as text for each file,
   aliases inside stored anchors re-resolved against the *latest* definition.
   Redefining an anchor retroactively changed the value of every other anchor
   that referenced it — surprising, and different from single-file YAML semantics.

## Considered Options

1. Keep live alias references in the registry and topologically sort entries when
   building the preamble. Fixes the forward-reference bug but keeps the retroactive
   redefinition behavior, and adds sorting complexity.
2. Expand aliases at collection time so registry entries are self-contained.

## Decision Outcome

**Option 2: when an anchor is collected into the registry, store a deep copy with all aliases expanded inline.**

Registry entries never reference each other, so:

* Preamble ordering is irrelevant — no forward references, no sorting.
* An anchor's value is fixed at the point it was defined. Redefining `&anchorA`
  later changes what `*anchorA` resolves to in subsequent files, but never changes
  the stored value of an `&anchorB` that referenced `*anchorA` before the
  redefinition (resolve-at-definition-time, matching single-file YAML semantics).

## More Information

The trade-off is preamble size: an alias serializes as `*name` (a few bytes), while an expanded copy inlines the full referenced value into each anchor that uses it. Deeply chained anchor references could grow the preamble multiplicatively, but real configs use flat config-block anchors and include file sizes are capped (see [2024-07-11 include file limits](2024-07-11_include_file_limits.md)).
