// Command gen writes the inventory's Go types from its descriptions.
//
// It is a program rather than a flag on schemagen because reading a
// description means running it: a description is a Go value, so the only thing
// that can read one is Go. Fifteen lines here is a smaller price than a
// generator that shells out to the toolchain to build a harness it wrote.
package main

import (
	"fmt"
	"os"

	"github.com/mbauer83/effect-golang-web/examples/inventory/definitions"
	"github.com/mbauer83/effect-golang-web/internal/schemagen"
)

func main() {
	written, err := schemagen.WriteBindings("inventory", definitions.Described()...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile("../"+schemagen.BindingsFileName, written, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gen: %v\n", err)
		os.Exit(1)
	}
}
