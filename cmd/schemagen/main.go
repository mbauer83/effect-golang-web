// Command schemagen writes the schema a struct already implies.
//
// It is a thin front to internal/schemagen, which holds the reading and the
// writing so that a test can regenerate in process and compare.
//
//	go run github.com/mbauer83/effect-golang-web/cmd/schemagen -package .
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mbauer83/effect-golang-web/internal/schemagen"
)

func main() {
	directory := flag.String("package", ".", "the package directory to read")
	output := flag.String("out", "",
		"where to write; defaults to "+schemagen.FileName+" in the package")
	flag.Parse()

	if err := run(*directory, *output); err != nil {
		fmt.Fprintf(os.Stderr, "schemagen: %v\n", err)
		os.Exit(1)
	}
}

func run(directory string, output string) error {
	rendered, err := schemagen.Generate(directory)
	if err != nil {
		return err
	}
	if output == "" {
		output = filepath.Join(directory, schemagen.FileName)
	}
	return os.WriteFile(output, rendered, 0o644)
}
