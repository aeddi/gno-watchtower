package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/aeddi/gno-watchtower/internal/scribe/config"
)

// generateConfigCmd writes the embedded default TOML to args[0]; if args[0] is
// "-", writes to `out` instead.
func generateConfigCmd(args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: scribe generate-config <output-file|->")
	}
	body, err := config.DefaultTOML()
	if err != nil {
		return err
	}
	if args[0] == "-" {
		_, err = out.Write(body)
		return err
	}
	if err := os.WriteFile(args[0], body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", args[0], err)
	}
	return nil
}
