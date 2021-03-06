// General Bazel-related warnings

package warn

import (
	"fmt"
	"strings"

	"github.com/bazelbuild/buildtools/build"
	"github.com/bazelbuild/buildtools/edit"
)

func constantGlobWarning(f *build.File, fix bool) []*Finding {
	findings := []*Finding{}

	if f.Type == build.TypeDefault {
		// Only applicable to Bazel files
		return findings
	}

	edit.EditFunction(f, "glob", func(call *build.CallExpr, stk []build.Expr) build.Expr {
		if len(call.List) == 0 {
			return nil
		}
		patterns, ok := call.List[0].(*build.ListExpr)
		if !ok {
			return nil
		}
		for _, expr := range patterns.List {
			str, ok := expr.(*build.StringExpr)
			if !ok {
				continue
			}
			if !strings.Contains(str.Value, "*") {
				start, end := str.Span()
				findings = append(findings, makeFinding(f, start, end, "constant-glob",
					"Glob pattern `"+str.Value+"` has no wildcard ('*'). "+
						"Constant patterns can be error-prone, move the file outside the glob.", true, nil))
				return nil // at most one warning per glob
			}
		}
		return nil
	})
	return findings
}

func nativeInBuildFilesWarning(f *build.File, fix bool) []*Finding {
	findings := []*Finding{}

	if f.Type != build.TypeBuild {
		return findings
	}

	build.Edit(f, func(expr build.Expr, stack []build.Expr) build.Expr {
		// Search for `native.xxx` nodes
		dot, ok := expr.(*build.DotExpr)
		if !ok {
			return nil
		}
		ident, ok := dot.X.(*build.Ident)
		if !ok || ident.Name != "native" {
			return nil
		}

		// TODO(https://github.com/bazelbuild/bazel/issues/7496): remove as soon as `existing_rule`
		// and `exsisting_rules` become available in BUILD files.
		if dot.Name == "existing_rule" || dot.Name == "existing_rules" {
			return nil
		}

		if fix {
			start, _ := dot.Span()
			return &build.Ident{
				Name:    dot.Name,
				NamePos: start,
			}
		}
		start, end := expr.Span()
		findings = append(findings,
			makeFinding(f, start, end, "native-build",
				`The "native" module shouldn't be used in BUILD files, its members are available as global symbols.`, true, nil))

		return nil
	})
	return findings
}

func nativePackageWarning(f *build.File, fix bool) []*Finding {
	findings := []*Finding{}

	if f.Type != build.TypeBzl {
		return findings
	}

	build.Walk(f, func(expr build.Expr, stack []build.Expr) {
		// Search for `native.package()` nodes
		call, ok := expr.(*build.CallExpr)
		if !ok {
			return
		}
		dot, ok := call.X.(*build.DotExpr)
		if !ok || dot.Name != "package" {
			return
		}
		ident, ok := dot.X.(*build.Ident)
		if !ok || ident.Name != "native" {
			return
		}

		start, end := expr.Span()
		findings = append(findings,
			makeFinding(f, start, end, "native-package",
				`"native.package()" shouldn't be used in .bzl files.`, true, nil))
	})
	return findings
}

func duplicatedNameWarning(f *build.File, fix bool) []*Finding {
	findings := []*Finding{}
	if f.Type == build.TypeBzl || f.Type == build.TypeDefault {
		// Not applicable to .bzl files.
		return findings
	}
	names := make(map[string]int) // map from name to line number
	msg := `A rule with name "%s" was already found on line %d. ` +
		`Even if it's valid for Blaze, this may confuse other tools. ` +
		`Please rename it and use different names.`

	for _, rule := range f.Rules("") {
		name := rule.ExplicitName()
		if name == "" {
			continue
		}
		start, end := rule.Call.Span()
		if nameNode := rule.Attr("name"); nameNode != nil {
			start, end = nameNode.Span()
		}
		if line, ok := names[name]; ok {
			findings = append(findings,
				makeFinding(f, start, end, "duplicated-name", fmt.Sprintf(msg, name, line), true, nil))
		} else {
			names[name] = start.Line
		}
	}
	return findings
}

func positionalArgumentsWarning(f *build.File, pkg string, stmt build.Expr) *Finding {
	msg := "All calls to rules or macros should pass arguments by keyword (arg_name=value) syntax."
	call, ok := stmt.(*build.CallExpr)
	if !ok {
		return nil
	}
	if id, ok := call.X.(*build.Ident); !ok || functionsWithPositionalArguments[id.Name] {
		return nil
	}
	for _, arg := range call.List {
		if _, ok := arg.(*build.AssignExpr); ok {
			continue
		}
		start, end := arg.Span()
		return makeFinding(f, start, end, "positional-args", msg, true, nil)
	}
	return nil
}

func argsKwargsInBuildFilesWarning(f *build.File, fix bool) []*Finding {
	findings := []*Finding{}

	if f.Type != build.TypeBuild {
		return findings
	}

	build.Walk(f, func(expr build.Expr, stack []build.Expr) {
		// Search for function call nodes
		call, ok := expr.(*build.CallExpr)
		if !ok {
			return
		}
		for _, param := range call.List {
			unary, ok := param.(*build.UnaryExpr)
			if !ok {
				continue
			}
			start, end := param.Span()
			switch unary.Op {
			case "*":
				findings = append(findings,
					makeFinding(f, start, end, "build-args-kwargs",
						`*args are not allowed in BUILD files.`, true, nil))
			case "**":
				findings = append(findings,
					makeFinding(f, start, end, "build-args-kwargs",
						`**kwargs are not allowed in BUILD files.`, true, nil))
			}
		}
	})
	return findings
}
