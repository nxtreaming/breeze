package main

import (
	"flag"
	"fmt"
	"go/token"
	"strings"
)

func runGenerate(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: breeze generate <handler|resource> <Name> [args...]")
	}
	kind, name, rest := args[0], args[1], args[2:]

	if !token.IsIdentifier(name) || !strings.HasPrefix(name, strings.ToUpper(name[:1])) {
		return fmt.Errorf("invalid name %q — must be an exported Go identifier (e.g. User)", name)
	}

	modulePath, err := currentModulePath()
	if err != nil {
		return err
	}

	switch kind {
	case "handler":
		return generateHandler(modulePath, name, rest)
	case "resource":
		return generateResource(modulePath, name, rest)
	default:
		return fmt.Errorf("unknown generator %q — must be handler or resource", kind)
	}
}

// action describes a single generated CRUD operation shared by both the
// handler and resource generators.
type action struct {
	Name       string // "list", "get", "create", "update", "delete"
	Method     string // breeze.GET, breeze.POST, ...
	PathSuffix string // "" or "/:id"
	FuncName   string
}

var allActions = []string{"list", "get", "create", "update", "delete"}

func actionsFor(name, plural string, requested []string) ([]action, error) {
	if len(requested) == 0 {
		requested = allActions
	}

	valid := make(map[string]bool, len(allActions))
	for _, a := range allActions {
		valid[a] = true
	}

	actions := make([]action, 0, len(requested))
	for _, r := range requested {
		r = strings.ToLower(strings.TrimSpace(r))
		if !valid[r] {
			return nil, fmt.Errorf("unknown method %q — must be one of: %s", r, strings.Join(allActions, ", "))
		}
		switch r {
		case "list":
			actions = append(actions, action{Name: r, Method: "breeze.GET", PathSuffix: "", FuncName: "List" + plural})
		case "get":
			actions = append(actions, action{Name: r, Method: "breeze.GET", PathSuffix: "/:id", FuncName: "Get" + name})
		case "create":
			actions = append(actions, action{Name: r, Method: "breeze.POST", PathSuffix: "", FuncName: "Create" + name})
		case "update":
			actions = append(actions, action{Name: r, Method: "breeze.PUT", PathSuffix: "/:id", FuncName: "Update" + name})
		case "delete":
			actions = append(actions, action{Name: r, Method: "breeze.DELETE", PathSuffix: "/:id", FuncName: "Delete" + name})
		}
	}
	return actions, nil
}

// splitFlagsAndPositional separates "--name=value" (or "-name=value") flag
// tokens from positional arguments, regardless of order. It requires flags
// with values to use the "=" form (as breeze's usage text documents) since
// the stdlib flag package otherwise stops parsing at the first positional
// token — which would break commands like `breeze new myapp --template=api`
// or `breeze generate resource User name:string --plural=people`.
func splitFlagsAndPositional(args []string) (flagArgs, positional []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			positional = append(positional, a)
		}
	}
	return flagArgs, positional
}

func parseMethodsFlag(fs *flag.FlagSet) *string {
	return fs.String("methods", strings.Join(allActions, ","), "comma-separated actions: list,get,create,update,delete")
}

func splitMethods(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
