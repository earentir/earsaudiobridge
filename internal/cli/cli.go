// Package cli provides shared command-line helpers.
package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// Execute runs root and exits non-zero on error.
func Execute(root *cobra.Command) {
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// ConfigPath resolves a config file path. Relative paths and bare filenames are
// read from the process working directory at startup.
func ConfigPath(flag, defaultName string) (string, error) {
	name := flag
	if name == "" {
		name = defaultName
	}
	if filepath.IsAbs(name) {
		return name, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return name, err
	}
	return filepath.Join(wd, name), nil
}
