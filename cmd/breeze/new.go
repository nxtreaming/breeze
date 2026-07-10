package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

//go:embed templates/api
var apiTemplateFS embed.FS

//go:embed templates/views
var viewsTemplateFS embed.FS

type newProjectData struct {
	Name   string
	Module string
}

func runNew(args []string) error {
	flags := flag.NewFlagSet("new", flag.ExitOnError)
	tmplName := flags.String("template", "api", "project template: api or views")
	module := flags.String("module", "", "Go module path (defaults to the project name)")

	flagArgs, positional := splitFlagsAndPositional(args)
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}

	if len(positional) != 1 {
		return fmt.Errorf("usage: breeze new <name> [--template=api|views] [--module=<import-path>]")
	}
	name := positional[0]

	var templateFS embed.FS
	var templateRoot string
	switch *tmplName {
	case "api":
		templateFS, templateRoot = apiTemplateFS, "templates/api"
	case "views":
		templateFS, templateRoot = viewsTemplateFS, "templates/views"
	default:
		return fmt.Errorf("unknown template %q — must be one of: api, views", *tmplName)
	}

	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("%s already exists", name)
	}

	modulePath := *module
	if modulePath == "" {
		modulePath = name
	}

	data := newProjectData{Name: name, Module: modulePath}

	if err := os.MkdirAll(name, 0o755); err != nil {
		return err
	}

	if err := renderTemplateTree(templateFS, templateRoot, name, data); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(name, "go.mod"), []byte(fmt.Sprintf("module %s\n\ngo 1.24.3\n", modulePath)), 0o644); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(name, registryFileName), []byte(registryTemplate(modulePath)), 0o644); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(name, "handlers"), 0o755); err != nil {
		return err
	}

	if err := runGoModTidy(name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: go mod tidy failed: %v\n", err)
	}

	fmt.Printf("Created %s (template: %s)\n\nNext steps:\n  cd %s\n  go run .\n", name, *tmplName, name)
	return nil
}

// renderTemplateTree walks every file under root in srcFS, rendering it
// through text/template with data and writing the result under destDir,
// preserving relative paths. Files named "*.tmpl" have that suffix stripped;
// a file named "gitignore.tmpl" becomes ".gitignore".
func renderTemplateTree(srcFS embed.FS, root, destDir string, data newProjectData) error {
	return fs.WalkDir(srcFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		destRel := rel
		isTemplate := filepath.Ext(rel) == ".tmpl"
		if isTemplate {
			destRel = rel[:len(rel)-len(".tmpl")]
			if filepath.Base(destRel) == "gitignore" {
				destRel = filepath.Join(filepath.Dir(destRel), ".gitignore")
			}
		}
		destPath := filepath.Join(destDir, destRel)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}

		content, err := srcFS.ReadFile(path)
		if err != nil {
			return err
		}

		if !isTemplate {
			return os.WriteFile(destPath, content, 0o644)
		}

		tmpl, err := template.New(rel).Parse(string(content))
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", path, err)
		}

		f, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer f.Close()

		return tmpl.Execute(f, data)
	})
}

func runGoModTidy(dir string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
