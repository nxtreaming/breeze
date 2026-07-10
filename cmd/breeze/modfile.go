package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// currentModulePath reads the module path out of go.mod in the current
// working directory. It returns an error that's safe to print directly to
// the user if go.mod is missing or malformed — generate commands need a
// Breeze project to work inside of.
func currentModulePath() (string, error) {
	f, err := os.Open("go.mod")
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no go.mod found in current directory — run this from the root of a Breeze project")
		}
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("go.mod has no module directive")
}
