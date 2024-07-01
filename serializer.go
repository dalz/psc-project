package main

import (
	"fmt"
	"io"
)

// Writes the network in text form to the provider io.Writer.
// The serialization format is described in the README.org file.
func (net network) serialize(w io.Writer) error {
	fmt.Fprintln(w, "digraph network {")

	// serialize each node
	for _, n := range net {
		n.serialize(w)
		fmt.Fprintln(w, "")
	}

	fmt.Fprintln(w, "}")

	return nil
}

func (n node) serialize(w io.Writer) {
	// format:
	// <id> [label=<name>] // <sendText> <sendInterval> <relayMode> <paused> <x> <y>
	fmt.Fprintf(w,
		"%d [label=\"%s\"] // \"%s\" %d %d %t %d %d\n",
		n.id,
		n.name,
		n.sendText,
		n.sendInterval.Milliseconds(),
		n.relayMode,
		n.paused,
		n.x, n.y)

	// also write all outgoing channels
	for _, o := range n.outs {
		fmt.Fprintf(w, "%d -> %d\n", n.id, o.dst)
	}
}
