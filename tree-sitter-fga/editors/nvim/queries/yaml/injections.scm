; extends
;
; Inject the FGA DSL into the inline model of a `.fga.yaml` store test:
;
;   model: |
;     model
;       schema 1.1
;     type user
;
; The `; extends` modeline has to be the first line of the file: Neovim stops
; scanning for modelines at the first line that is not a comment, and without
; it this file would replace the yaml injections rather than add to them.
;
; The offset moves the start one column past the `|` indicator, which the
; `block_scalar` node includes. Offsetting by a row instead does not work:
; `#offset!` keeps the original start column, which on the next line is
; already past the end of a short one, so the region would begin at `schema`
; rather than at `model`. Starting just after the `|` leaves a newline and the
; block's indentation at the head of the region, and the grammar treats both
; as whitespace.

(block_mapping_pair
  key: (flow_node) @_key
  value: (block_node (block_scalar) @injection.content)
  (#eq? @_key "model")
  (#set! injection.language "fga")
  (#offset! @injection.content 0 1 0 0))
