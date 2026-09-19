# Name resolution and the index

## What bounds a name

A workspace can hold several unrelated models, so resolving against everything
would call a name defined in the wrong store "known".

An `fga.mod` draws the boundary exactly, and is the boundary used. A module
that no `fga.mod` claims has no boundary to draw and falls back to the whole
workspace — better than reporting every sibling module's types as unknown.

## Completeness

Referenced files are loaded from disk and followed to a fixed point: opening a
module finds its `fga.mod`, which names the siblings that complete the set.
The walk sweeps every known document each round rather than following one
document's references, which is what makes it transitive without tracking
which file introduced which reference.

This matters for any client that syncs only the files it opens rather than
scanning the workspace. Without it the server sees a truncated module set, and
every name defined in a sibling resolves to "unknown relation" — a correct
line reported as an error.

## Lifetime

A parse tree is C memory owned by its document. Two rules follow, and both
have already caused silent failures:

- **A `View`, and every document reached through it, is valid only inside the
  `Read` callback.** Replacing a document frees the tree behind the old one.
  `Index.Put` therefore returns nothing: handing back the stored document
  would invite a caller to hold it across the next `Put`, after which
  `Root()` returns nil and the tree-sitter pass silently produces nothing.
- **Parsers are built per call, not pooled.** go-tree-sitter registers no
  finalizers, so a `sync.Pool` — which drops its contents on any GC cycle —
  leaks the C parser behind every entry it discards, unbounded over the life
  of a server.

An out-of-order `didChange` must not install a stale buffer over a newer one,
so `Put` compares versions. Version 0 means "loaded from disk", which anything
synced wins over.

## URIs

`url.Parse` already decodes the path. Unescaping it a second time turns a
literal percent into an escape: `/tmp/100%.fga` arrives as
`file:///tmp/100%25.fga` and decodes twice into an error, leaving the file
silently unanalysed with no diagnostic anywhere.
