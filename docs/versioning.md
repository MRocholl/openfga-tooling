# Versioning

## Pins track the CLI

The server's diagnostics are meant to say exactly what `fga model validate`
says. A model the editor accepts and CI rejects is worse than no diagnostics
at all.

So `fga-lsp` pins the same `github.com/openfga/language/pkg/go` and
`github.com/openfga/openfga` versions that the `fga` release pins, and the
conformance corpus is vendored from the `openfga/language` commit that pin
names. Three things move together; bump them together.

Read the CLI's own `go.mod` to find them — the released tag of
`language/pkg/go` is usually behind what the CLI actually uses, so following
semver here gives the wrong answer.

## When the corpus disagrees

The corpus tracks `openfga/language`, which moves ahead of the server release
the CLI embeds. A case the corpus calls valid can still be rejected by the
pinned typesystem.

**The pinned server wins**, and the case is skipped with the reason recorded
in `aheadOfTheServer` in `fga-lsp/internal/conformance`. Before adding an
entry, confirm against the real binary that `fga model validate` reports what
this server reports — agreeing with the CLI is the whole point, so an entry
that has not been checked against it is worthless. Drop entries when a bump
makes them disagree.
