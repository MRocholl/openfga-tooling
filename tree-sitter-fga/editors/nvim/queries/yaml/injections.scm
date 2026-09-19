; Inject the FGA DSL into the inline model of a `.fga.yaml` store test:
;
;   model: |
;     model
;       schema 1.1
;     type user
;
; Install alongside your yaml queries (`queries/yaml/injections.scm`) with
; `; extends` at the top so it adds to, rather than replaces, the defaults.

; extends

(block_mapping_pair
  key: (flow_node) @_key
  value: (block_node (block_scalar) @injection.content)
  (#eq? @_key "model")
  (#set! injection.language "fga")
  (#offset! @injection.content 0 1 0 0))
