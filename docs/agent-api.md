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

Output is plain text, not JSON. Paths are relative to the workspace root and
lines are one-based, so an agent can quote a location straight into a reply or
paste it into an editor. Excerpts come with the answer, which saves the agent
a follow-up read of the file.

## Queries

| Shell | MCP tool | Answers |
| --- | --- | --- |
| `check [file]` | `fga_check` | is the model valid, and if not, where exactly |
| `overview` | `fga_overview` | what is in this workspace |
| `describe <type>` | `fga_describe` | what relations does this type have, merged across modules |
| `definition <name>` | `fga_definition` | where is this name defined |
| `references <name>` | `fga_references` | what uses this name, models and store tests alike |
| `search <query>` | `fga_search` | which names look like this |

Every shell command takes `-C <dir>` to pick the workspace, defaulting to the
current directory. `check` exits 1 when it reports something, 0 when clean.

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
