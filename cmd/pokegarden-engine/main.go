// Command pokegarden-engine is a drop-in replacement for the former .NET
// PKHeX bridge: it reads one JSON request on stdin and writes one JSON
// response on stdout (exit status 1 with {"error": "..."} on failure).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/8bit-necro/go-pokeedit/engine"
)

func main() {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stdout, `{"error":"could not read request"}`)
		os.Exit(1)
	}
	out, ok := engine.Run(in)
	os.Stdout.Write(append(out, '\n'))
	if !ok {
		os.Exit(1)
	}
}
