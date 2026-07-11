package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
)

const middlewareImport = `middleware "github.com/nelthaarion/breeze/middlewares"`
const swaggerImport = `"github.com/nelthaarion/breeze/swagger"`

func generateResource(modulePath, name string, args []string) error {
	fs := flag.NewFlagSet("generate resource", flag.ExitOnError)
	pluralOverride := fs.String("plural", "", "override the pluralized resource name (e.g. --plural=people)")
	force := fs.Bool("force", false, "overwrite an existing handler file")

	flagArgs, positional := splitFlagsAndPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	fields, err := parseFields(positional)
	if err != nil {
		return err
	}
	if len(fields) == 0 {
		return fmt.Errorf("usage: breeze generate resource <Name> field:type [field:type ...]")
	}

	plural := *pluralOverride
	if plural == "" {
		plural = pluralize(name)
	}
	pathBase := "/" + strings.ToLower(plural)

	actions, err := actionsFor(name, plural, allActions)
	if err != nil {
		return err
	}

	if err := writeResourceHandlerFile(name, plural, fields, *force); err != nil {
		return err
	}

	docArgs := make([]string, len(actions))
	for i, a := range actions {
		docArgs[i] = routeDoc(a, name, plural, pathBase+a.PathSuffix)
	}

	return registerActionRoutes(modulePath, name, pathBase, actions, docArgs, middlewareImport, swaggerImport)
}

func writeResourceHandlerFile(name, plural string, fields []field, force bool) error {
	if err := os.MkdirAll("handlers", 0o755); err != nil {
		return err
	}

	path := filepath.Join("handlers", strings.ToLower(name)+".go")
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("%s already exists — pass --force to overwrite", path)
	}

	nameLower := strings.ToLower(name)

	var b strings.Builder
	b.WriteString("package handlers\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"sync\"\n")
	if usesTime(fields) {
		b.WriteString("\t\"time\"\n")
	}
	b.WriteString("\n\t\"github.com/nelthaarion/breeze\"\n")
	b.WriteString(")\n\n")

	writeStruct(&b, "Create"+name+"Request", fields)
	writeStruct(&b, "Update"+name+"Request", fields)
	writeResponseStruct(&b, name+"Response", fields)

	fmt.Fprintf(&b, "type %sListResponse struct {\n", name)
	fmt.Fprintf(&b, "\t%s []%sResponse `json:\"%s\"`\n", plural, name, strings.ToLower(plural))
	b.WriteString("\tTotal int `json:\"total\"`\n")
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "type %sPathParams struct {\n", name)
	b.WriteString("\tID string `json:\"id\" description:\"" + name + " ID\"`\n")
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "var (\n\t%sMu sync.RWMutex\n\t%sStore = []%sResponse{}\n\t%sNextID = 1\n)\n\n",
		nameLower, nameLower, name, nameLower)

	fmt.Fprintf(&b, "// List%s handles GET /%s.\nfunc List%s(ctx *breeze.Context) {\n", plural, strings.ToLower(plural), plural)
	fmt.Fprintf(&b, "\t%sMu.RLock()\n\tdefer %sMu.RUnlock()\n", nameLower, nameLower)
	fmt.Fprintf(&b, "\tctx.JSON(%sListResponse{%s: %sStore, Total: len(%sStore)})\n}\n\n", name, plural, nameLower, nameLower)

	fmt.Fprintf(&b, "// Get%s handles GET /%s/:id.\nfunc Get%s(ctx *breeze.Context) {\n", name, strings.ToLower(plural), name)
	fmt.Fprintf(&b, "\tid := ctx.GetParam(\"id\")\n\t%sMu.RLock()\n\tdefer %sMu.RUnlock()\n", nameLower, nameLower)
	fmt.Fprintf(&b, "\tfor _, item := range %sStore {\n\t\tif item.ID == id {\n\t\t\tctx.JSON(item)\n\t\t\treturn\n\t\t}\n\t}\n", nameLower)
	b.WriteString("\tctx.Status(404)\n\tctx.JSON(map[string]string{\"error\": \"not found\"})\n}\n\n")

	fmt.Fprintf(&b, "// Create%s handles POST /%s.\nfunc Create%s(ctx *breeze.Context) {\n", name, strings.ToLower(plural), name)
	fmt.Fprintf(&b, "\tvar req Create%sRequest\n\tif err := json.Unmarshal(ctx.Req.Body, &req); err != nil {\n", name)
	b.WriteString("\t\tctx.Status(400)\n\t\tctx.JSON(map[string]string{\"error\": \"invalid body\"})\n\t\treturn\n\t}\n\n")
	fmt.Fprintf(&b, "\t%sMu.Lock()\n\tid := fmt.Sprintf(\"%%d\", %sNextID)\n\t%sNextID++\n", nameLower, nameLower, nameLower)
	fmt.Fprintf(&b, "\titem := %sResponse{ID: id", name)
	for _, f := range fields {
		fmt.Fprintf(&b, ", %s: req.%s", f.Name, f.Name)
	}
	b.WriteString("}\n")
	fmt.Fprintf(&b, "\t%sStore = append(%sStore, item)\n\t%sMu.Unlock()\n\n", nameLower, nameLower, nameLower)
	b.WriteString("\tctx.Status(201)\n\tctx.JSON(item)\n}\n\n")

	fmt.Fprintf(&b, "// Update%s handles PUT /%s/:id.\nfunc Update%s(ctx *breeze.Context) {\n", name, strings.ToLower(plural), name)
	fmt.Fprintf(&b, "\tvar req Update%sRequest\n\tif err := json.Unmarshal(ctx.Req.Body, &req); err != nil {\n", name)
	b.WriteString("\t\tctx.Status(400)\n\t\tctx.JSON(map[string]string{\"error\": \"invalid body\"})\n\t\treturn\n\t}\n\n")
	b.WriteString("\tid := ctx.GetParam(\"id\")\n")
	fmt.Fprintf(&b, "\t%sMu.Lock()\n\tdefer %sMu.Unlock()\n", nameLower, nameLower)
	fmt.Fprintf(&b, "\tfor i, item := range %sStore {\n\t\tif item.ID == id {\n", nameLower)
	for _, f := range fields {
		fmt.Fprintf(&b, "\t\t\t%sStore[i].%s = req.%s\n", nameLower, f.Name, f.Name)
	}
	fmt.Fprintf(&b, "\t\t\tctx.JSON(%sStore[i])\n\t\t\treturn\n\t\t}\n\t}\n", nameLower)
	b.WriteString("\tctx.Status(404)\n\tctx.JSON(map[string]string{\"error\": \"not found\"})\n}\n\n")

	fmt.Fprintf(&b, "// Delete%s handles DELETE /%s/:id.\nfunc Delete%s(ctx *breeze.Context) {\n", name, strings.ToLower(plural), name)
	b.WriteString("\tid := ctx.GetParam(\"id\")\n")
	fmt.Fprintf(&b, "\t%sMu.Lock()\n\tdefer %sMu.Unlock()\n", nameLower, nameLower)
	fmt.Fprintf(&b, "\tfor i, item := range %sStore {\n\t\tif item.ID == id {\n\t\t\t%sStore = append(%sStore[:i], %sStore[i+1:]...)\n\t\t\tctx.Status(204)\n\t\t\treturn\n\t\t}\n\t}\n",
		nameLower, nameLower, nameLower, nameLower)
	b.WriteString("\tctx.Status(404)\n\tctx.JSON(map[string]string{\"error\": \"not found\"})\n}\n")

	formatted, err := format.Source([]byte(addJSONImport(b.String())))
	if err != nil {
		return fmt.Errorf("formatting %s: %w", path, err)
	}
	return os.WriteFile(path, formatted, 0o644)
}

// addJSONImport inserts encoding/json into the import block generated above.
// Kept separate from the main builder so the import list stays easy to read.
func addJSONImport(src string) string {
	return strings.Replace(src, "\t\"fmt\"\n", "\t\"encoding/json\"\n\t\"fmt\"\n", 1)
}

func writeStruct(b *strings.Builder, typeName string, fields []field) {
	fmt.Fprintf(b, "type %s struct {\n", typeName)
	for _, f := range fields {
		fmt.Fprintf(b, "\t%s %s `json:\"%s\"`\n", f.Name, f.Type, f.JSON)
	}
	b.WriteString("}\n\n")
}

func writeResponseStruct(b *strings.Builder, typeName string, fields []field) {
	fmt.Fprintf(b, "type %s struct {\n", typeName)
	b.WriteString("\tID string `json:\"id\"`\n")
	for _, f := range fields {
		fmt.Fprintf(b, "\t%s %s `json:\"%s\"`\n", f.Name, f.Type, f.JSON)
	}
	b.WriteString("}\n\n")
}

// routeDoc renders the middleware.DocXXX(...) call for a single action,
// wiring up swagger.RouteDoc from the generated request/response types.
func routeDoc(a action, name, plural, path string) string {
	tags := fmt.Sprintf("[]string{%q}", plural)
	switch a.Name {
	case "list":
		return fmt.Sprintf(`middleware.DocGET(%q, swagger.RouteDoc{
	Title:        %q,
	Tags:         %s,
	Output:       handlers.%sListResponse{},
	OutputStatus: 200,
})`, path, "List "+plural, tags, name)
	case "get":
		return fmt.Sprintf(`middleware.DocGET(%q, swagger.RouteDoc{
	Title: %q,
	Tags:  %s,
	Input: []swagger.InputGroup{
		{Type: swagger.InputParams, Fields: handlers.%sPathParams{}},
	},
	Output: handlers.%sResponse{},
})`, path, "Get "+name+" by ID", tags, name, name)
	case "create":
		return fmt.Sprintf(`middleware.DocPOST(%q, swagger.RouteDoc{
	Title: %q,
	Tags:  %s,
	Input: []swagger.InputGroup{
		{Type: swagger.InputBody, Fields: handlers.Create%sRequest{}, Required: true},
	},
	Output:       handlers.%sResponse{},
	OutputStatus: 201,
})`, path, "Create "+name, tags, name, name)
	case "update":
		return fmt.Sprintf(`middleware.DocPUT(%q, swagger.RouteDoc{
	Title: %q,
	Tags:  %s,
	Input: []swagger.InputGroup{
		{Type: swagger.InputParams, Fields: handlers.%sPathParams{}},
		{Type: swagger.InputBody, Fields: handlers.Update%sRequest{}, Required: true},
	},
	Output: handlers.%sResponse{},
})`, path, "Update "+name, tags, name, name, name)
	case "delete":
		return fmt.Sprintf(`middleware.DocDELETE(%q, swagger.RouteDoc{
	Title: %q,
	Tags:  %s,
	Input: []swagger.InputGroup{
		{Type: swagger.InputParams, Fields: handlers.%sPathParams{}},
	},
	Output:            struct{}{},
	OutputStatus:      204,
	OutputDescription: %q,
})`, path, "Delete "+name, tags, name, name+" deleted")
	}
	return ""
}
