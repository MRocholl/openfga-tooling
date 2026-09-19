; OpenFGA DSL -- highlights

; ---------------------------------------------------------------- declarations

(model_header ["model" "schema"] @keyword)
(schema_version) @number

(module_header "module" @keyword.import)
(module_header name: (identifier) @module)

(type_definition ["extend" "type"] @keyword.type)
(type_definition name: (identifier) @type.definition)

(relations_block "relations" @keyword)
(relation_definition "define" @keyword.function)
(relation_definition name: (identifier) @function.method)

; ------------------------------------------------------------------- operators

[
  "or"
  "and"
  "from"
  "with"
] @keyword.operator

(but_not) @keyword.operator

; -------------------------------------------------------------- relation terms

(computed_relation relation: (identifier) @variable.member)

(tupleset_relation relation: (identifier) @variable.member)
(tupleset_relation tupleset: (identifier) @variable.member)

(type_restriction type: (identifier) @type)
(relation_suffix "#" @punctuation.special)
(relation_suffix relation: (identifier) @variable.member)
(wildcard) @character.special
(condition_suffix condition: (identifier) @function.call)
(inline_expression) @function.builtin

; ------------------------------------------------------------------ conditions

(condition "condition" @keyword.function)
(condition name: (identifier) @function)
(parameter name: (identifier) @variable.parameter)
(scalar_type) @type.builtin
(parameter_type ["map" "list"] @type.builtin)

; ------------------------------------------------------------------------- CEL

(call_expression
  function: (identifier) @function.call)

(member_expression property: (identifier) @variable.member)

(map_entry key: (identifier) @property)

[
  "=="
  "!="
  "<"
  "<="
  ">"
  ">="
  "&&"
  "||"
  "!"
  "+"
  "-"
  "*"
  "/"
  "%"
  "?"
] @operator

"in" @keyword.operator

(boolean) @boolean
(null) @constant.builtin
(number) @number
(string) @string

((identifier) @variable
  (#set! "priority" 90))

; ---------------------------------------------------------------- punctuation

[ "(" ")" "[" "]" "{" "}" "<" ">" ] @punctuation.bracket
[ "," ":" "." ] @punctuation.delimiter

(comment) @comment @spell
