package feature

import (
	"fmt"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/analysis"
)

// Hover explains what is under the cursor.
func Hover(v *analysis.View, doc *analysis.Document, pos protocol.Position) *protocol.Hover {
	target := TargetAt(v, doc, pos)
	if !target.Found() {
		return nil
	}

	scope := analysis.ScopeFor(v, doc)

	content := hoverContent(scope, target)
	if content == "" {
		return nil
	}

	rng := target.Range

	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: content},
		Range:    &rng,
	}
}

func hoverContent(scope analysis.Scope, target Target) string {
	switch target.Kind {
	case TargetType:
		return hoverType(scope, target.Name)
	case TargetRelation:
		return hoverRelation(scope, target)
	case TargetCondition:
		return hoverCondition(scope, target.Name)
	case TargetFileRef:
		return "`" + target.Path + "`"
	case TargetModule, TargetNone:
	}

	return ""
}

func hoverType(scope analysis.Scope, name string) string {
	decls := scope.TypeDecls(name)
	if len(decls) == 0 {
		return fmt.Sprintf("`type %s`\n\n_not declared in this model_", name)
	}

	var b strings.Builder

	fmt.Fprintf(&b, "```fga\ntype %s\n```\n", name)

	if modules := modulesOf(decls); modules != "" {
		fmt.Fprintf(&b, "\nDeclared in %s.\n", modules)
	}

	if doc := firstDoc(decls); doc != "" {
		b.WriteString("\n" + doc + "\n")
	}

	relations := scope.RelationsOf(name)
	if len(relations) == 0 {
		return b.String()
	}

	fmt.Fprintf(&b, "\n**%d relation%s**\n\n```fga\n", len(relations), plural(len(relations)))

	for _, rel := range relations {
		fmt.Fprintf(&b, "  define %s: %s\n", rel.Name, rel.Expr)
	}

	b.WriteString("```\n")

	return b.String()
}

func hoverRelation(scope analysis.Scope, target Target) string {
	var decls []*analysis.RelationDecl

	for _, owner := range target.OwnerTypes {
		decls = append(decls, scope.RelationDecls(owner, target.Name)...)
	}

	if len(decls) == 0 {
		owners := strings.Join(target.OwnerTypes, ", ")
		if owners == "" {
			owners = "any type in scope"
		}

		return fmt.Sprintf("`%s`\n\n_not defined on %s_", target.Name, owners)
	}

	var b strings.Builder

	b.WriteString("```fga\n")

	for _, decl := range decls {
		fmt.Fprintf(&b, "type %s\n  relations\n    define %s: %s\n", decl.TypeName, decl.Name, decl.Expr)
	}

	b.WriteString("```\n")

	if module := moduleOf(decls[0]); module != "" {
		fmt.Fprintf(&b, "\nDefined in %s.\n", module)
	}

	if decls[0].Doc != "" {
		b.WriteString("\n" + decls[0].Doc + "\n")
	}

	return b.String()
}

func hoverCondition(scope analysis.Scope, name string) string {
	decls := scope.ConditionDecls(name)
	if len(decls) == 0 {
		return fmt.Sprintf("`condition %s`\n\n_not declared in this model_", name)
	}

	decl := decls[0]

	params := make([]string, 0, len(decl.Params))
	for _, param := range decl.Params {
		params = append(params, param.Name+": "+param.Type)
	}

	var b strings.Builder

	fmt.Fprintf(&b, "```fga\ncondition %s(%s)\n```\n", decl.Name, strings.Join(params, ", "))

	if decl.Doc != "" {
		b.WriteString("\n" + decl.Doc + "\n")
	}

	return b.String()
}

func modulesOf(decls []*analysis.TypeDecl) string {
	var (
		seen    = map[string]struct{}{}
		modules []string
	)

	for _, decl := range decls {
		label := decl.Module
		if label == "" {
			label = "the model"
		} else {
			label = "module `" + label + "`"
		}

		if decl.Extend {
			label += " (extend)"
		}

		if _, ok := seen[label]; ok {
			continue
		}

		seen[label] = struct{}{}

		modules = append(modules, label)
	}

	return strings.Join(modules, ", ")
}

func moduleOf(decl *analysis.RelationDecl) string {
	if decl == nil {
		return ""
	}

	return "`" + shortName(string(decl.URI)) + "`"
}

func shortName(uri string) string {
	if idx := strings.LastIndex(uri, "/"); idx >= 0 {
		return uri[idx+1:]
	}

	return uri
}

func firstDoc(decls []*analysis.TypeDecl) string {
	for _, decl := range decls {
		if decl.Doc != "" {
			return decl.Doc
		}
	}

	return ""
}

func plural(n int) string {
	if n == 1 {
		return ""
	}

	return "s"
}
