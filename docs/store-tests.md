# Store tests

`.fga.yaml` files are checked against the model they point at: tuple and check
syntax, and whether every type and relation named is in that model, with a
nearest-name suggestion when it is not. In a model full of near-identical
`can_*` relations the suggestion is most of the value of the diagnostic.

## Inline models

A store may carry its model in a `model: |` block instead of naming a file.
That block has no file name, so **its kind has to be stated rather than
inferred**. Deriving it from the store's own URI — even with a fragment
appended — reads the DSL back as a store test, leaving every type in it
undeclared and every name in the file reported as unknown.

The document built for that block exists for one call and owns a parse tree
that nothing else will free, so the scope that holds it closes it.

## Finding a range in YAML

Ranges are found by searching the line **from the column yaml.v3 reports**,
not from the start of it. A flow mapping puts several scalars on one line, and
a name that is a suffix of an earlier one would otherwise underline the wrong
text: in `{can_read: true, read: true}` a search for `read` finds the one
inside `can_read`.

The reported column points at the opening quote for a quoted scalar, so
searching from there rather than taking it literally lands inside the quotes
either way.

YAML 1.1 booleans admit more spellings than Go's `strconv` does; `yes` and
`on` are true.

## Containment

`fga` v0.8.0 refuses a `model_file`, `tuple_file` or `tuple_files` reference
that resolves outside the test file's own directory unless
`--allow-external-files` is passed. A layout with tests in a subdirectory
pointing at `../fga.mod` needs that flag. The server does not enforce
containment — it is reading files in an editor, not running untrusted input.
