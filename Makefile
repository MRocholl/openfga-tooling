# Build both halves into the layout the editor wiring expects.
#
#   bin/fga-lsp          the language server
#   nvim/parser/fga.so   the tree-sitter parser, found by Neovim on rtp
#   nvim/queries/fga/    the highlight queries, copied from the grammar

GRAMMAR := tree-sitter-fga
SERVER  := fga-lsp

.PHONY: all
all: bin/$(SERVER) nvim/parser/fga.so nvim/queries/fga/highlights.scm nvim/queries/yaml/injections.scm

bin/$(SERVER): $(shell find $(SERVER) -name '*.go' 2>/dev/null) $(SERVER)/go.mod
	cd $(SERVER) && go build -o ../bin/$(SERVER) .

nvim/parser/fga.so: $(GRAMMAR)/src/parser.c
	@mkdir -p nvim/parser
	tree-sitter build -o $(CURDIR)/nvim/parser/fga.so $(CURDIR)/$(GRAMMAR)

# Neovim wants queries/<lang>/<name>.scm; the tree-sitter CLI wants
# queries/<name>.scm. The grammar keeps the CLI layout and the copies are
# generated, so the two cannot drift.
#
# The yaml injection is a separate target on purpose: folding it into the
# rule above left it stale whenever only it had changed.
nvim/queries/fga/highlights.scm: $(wildcard $(GRAMMAR)/queries/*.scm)
	@mkdir -p nvim/queries/fga
	cp $(GRAMMAR)/queries/*.scm nvim/queries/fga/

nvim/queries/yaml/injections.scm: $(GRAMMAR)/editors/nvim/queries/yaml/injections.scm
	@mkdir -p nvim/queries/yaml
	cp $< $@

.PHONY: generate
generate:
	cd $(GRAMMAR) && tree-sitter generate

.PHONY: test
test:
	cd $(GRAMMAR) && tree-sitter test
	cd $(SERVER) && go test ./...
	cd $(SERVER) && go vet ./...

.PHONY: clean
clean:
	rm -rf bin nvim/parser nvim/queries/fga nvim/queries/yaml
