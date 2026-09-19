# Agent API

The same analysis the language server provides, reachable two ways:

```sh
fga-lsp check -C database/authz          # a shell command
fga-lsp mcp database/authz               # MCP over stdin/stdout
```

## Which one

**Prefer the shell commands.** An agent with a shell already has them: no
configuration, no server process, and nothing sitting in the context window
until the moment a query is made. They pipe, they compose, and `check` exits
non-zero when it found something, so the same call works in CI.

MCP earns its place where there is no shell — Claude Desktop, the web app,
other MCP clients — and where per-tool permissions are wanted. Both run the
same code over the same workspace; only the transport differs.

## Why it is not a thin LSP wrapper

LSP is machine-facing and position-addressed: every request carries a URI, a
zero-based line and a character offset, and every answer comes back as
`targetSelectionRange`, `originSelectionRange`, `LocationLink`. An agent
almost never has a position. It has a *name* — `document#can_view` — read out
of a diagnostic, a review comment or a ticket.

So the tools are name-addressed, and the answers are the semantic result
rather than the protocol's shape:

```
name      -> where it is defined
name      -> where it is used
type      -> what it can do
workspace -> what is wrong with it
```

Paths are relative to the workspace root and lines are one-based, so an agent
can quote a location straight into a reply or paste it into an editor.

## Formats

`-format` picks the shape. MCP always uses `agent`, since nothing there is
read by a person.

**`agent`** is one finding per line, `path:line:column: kind: text` -- what a
compiler emits, so every editor, `grep` and agent already parses it:

```
laboratory.fga:21:12: definition: define laboratory_access: [role#assignee]
integrations.fga:13:41: use: define can_read_kvdt_configuration: laboratory_access
account.fga.yaml:32:15: test: relation: laboratory_access
31 references in 18 files
```

That flatness is the point. The text form groups by file and caps the lines
per file, which reads well and filters badly; the agent form sorts by path and
line so two runs are diffable, and leaves the filtering to the caller:

```sh
fga-lsp references -format agent 'organization#laboratory_access' | grep ': definition:'
```

The `kind` is what an agent needs before it edits: a `definition` is the thing
itself, a `use` is a model that depends on it, a `test` is an assertion that
will fail if it changes.

**`json`** is the same data structured, for a caller that would otherwise
parse the text.

**`text`** is the default, for a person: grouped by file, with excerpts and a
caret under the offending name.

## Completeness

The agent and JSON forms return **everything** by default. A truncated
reference list is worse than a long one when the question is whether a
relation is safe to remove, so cutting it is opt-in through `-limit`, and what
was left out is stated:

```
31 references in 18 files, 25 omitted by --limit
```

The text form does abbreviate, because a person scanning a long list wants the
shape rather than every line; past roughly a hundred hits it collapses to
per-file counts.

## Queries

| Shell | MCP tool | Answers |
| --- | --- | --- |
| `check [file]` | `fga_check` | is the model valid, and if not, where exactly |
| `overview` | `fga_overview` | what is in this workspace |
| `describe <type>` | `fga_describe` | what relations does this type have, merged across modules |
| `definition <name>` | `fga_definition` | where is this name defined |
| `references <name>` | `fga_references` | what uses this name, models and store tests alike |
| `search <query>` | `fga_search` | which names look like this |

Every shell command takes `-C <dir>` to pick the workspace (default: the
current directory), `-format text|agent|json`, and `-limit N` to cap results.
`check` exits 1 when it reports something, 0 when clean.

## Compaction

Two rules keep answers readable when a model is large.

**Excerpts are capped per file and files are capped per answer**, with the
remainder reported as a count rather than dropped silently.

**A long reference list collapses to per-file counts.** Past roughly a hundred
hits the individual lines stop being useful and the shape is the answer, so
`fga_references` returns the files ranked by count and invites a narrower
query. This mirrors how a person reads a large result: the distribution
first, then one file.

## Wiring

This repository ships a `.mcp.json`, so Claude Code picks the server up when
run here. For another project, point it at the binary and the model directory:

```json
{
  "mcpServers": {
    "fga": {
      "command": "/path/to/fga-lsp",
      "args": ["mcp", "database/authz"]
    }
  }
}
```

The workspace is re-scanned on every `fga_check` and `fga_overview`, so edits
made between calls are picked up without a restart.
