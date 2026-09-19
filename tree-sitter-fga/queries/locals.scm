; OpenFGA DSL -- local scopes
;
; A condition is the only construct with genuinely local names: its parameters
; are visible in its body and nowhere else.

(source_file) @local.scope
(condition) @local.scope

(parameter name: (identifier) @local.definition.parameter)

(condition name: (identifier) @local.definition.function)

(member_expression property: (identifier)) @local.reference
(identifier) @local.reference
