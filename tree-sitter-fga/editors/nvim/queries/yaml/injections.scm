; extends
;
; Injects the FGA DSL into a store test's `model: |` block.
; The modeline must be line 1 and the offset must be a column, not a row --
; see docs/editor-integration.md.

(block_mapping_pair
  key: (flow_node) @_key
  value: (block_node (block_scalar) @injection.content)
  (#eq? @_key "model")
  (#set! injection.language "fga")
  (#offset! @injection.content 0 1 0 0))
