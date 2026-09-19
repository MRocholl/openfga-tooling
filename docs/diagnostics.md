# Diagnostics

## Three passes, cheapest first

Each pass gates the next, because a later one is only meaningful on input the
earlier one accepted.

1. **The tree-sitter tree.** Error and missing nodes. This is the only pass
   with an answer for a half-typed buffer, and its ranges are the tightest.
2. **The reference ANTLR parser**, run per module file. It catches what the
   looser grammar lets through. Running it per file is what puts the error in
   the module that caused it: a syntax failure discovered inside
   `TransformModuleFilesToModel` arrives as a type with no `File` field, and
   would otherwise be reported against the `fga.mod` that happens to list the
   module, at line 1, carrying a raw `syntax error at line=…` string.
3. **openfga's typesystem**, the same check `fga model validate` runs.

Name resolution runs even when the DSL does not compile. An unknown relation
is exactly what a half-finished edit produces, and pointing at the word is
more useful than waiting for the file to become valid.

## Upstream wording

Messages and ranges match `openfga/language`'s own validator, case for case,
against the vendored corpus. This is not pedantry: it means a squiggle in the
editor says the same thing as `fga model validate` in the terminal and as the
official editor plugins, so nobody learns two vocabularies for one mistake.

The Go package carries only syntactic validation. The semantic checks upstream
implements in JS and Java are reimplemented here to the same messages:
duplicate types, restrictions and partials; reserved `self` and `this`; the
type and relation naming rules; undefined and unused conditions; undefined
types and relations; and the tupleset rules below.

Two details the corpus pins down that are easy to get wrong:

- **A duplicate is reported at its first occurrence**, not its second.
- **A tupleset relation must name plain types.** `[folder, folder#parent]` or
  `[document, document:*]` disqualifies a relation from appearing to the right
  of `from`: a tupleset has to name objects to walk to, not sets of users.
  Being a direct assignment is necessary but not sufficient.
- **One error per candidate target type.** `u3 from children` where `children:
  [child1, child2]` and `u3` exists on neither yields two diagnostics at the
  same range, one naming each type.

## Placing a typesystem error

Most of the typesystem's failures are plain `fmt.Errorf` values with no
structured fields, so the type and relation are recovered from the message
text. Without that, the majority of them land on line 1 of the file.

Anything name resolution already reported is suppressed, keyed by
`type#relation`. The typesystem repeats those findings in its own words and,
having no source position for them, would pin the repeat to the top of the
file next to the precise one.

## Coordinates

Three systems are in play: byte offsets (tree-sitter and Go), row and
byte-column points (tree-sitter), and LSP positions, whose character is a
count of UTF-16 code units. The distinction only shows up once a line holds a
non-ASCII rune, which in practice means a comment — and a comment sits on the
same line as the code it documents often enough to matter.

Both `openfga/language` transformers report zero-based lines and columns,
despite doc comments upstream claiming otherwise. ANTLR counts columns in
runes, not bytes; DSL identifiers are ASCII, so this only bites on a
non-ASCII byte earlier in the same line.
