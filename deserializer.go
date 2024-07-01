package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
)

type endpoints struct{ src, dst nodeid }

// return true if the next byte in the reader is c, without consuming it
func peek(r *bufio.Reader, c byte) bool {
	bs, err := r.Peek(1)
	if err != nil || bs[0] != c {
		return false
	}

	return true
}

// same as peek, but also consume the byte if it matches the argument
func consume(r *bufio.Reader, c byte) bool {
	if peek(r, c) {
		r.Discard(1)
		return true
	} else {
		return false
	}
}

// consume spaces and newlines from the reader,
// stop at the first non-whitespace character
func skipEmptyLines(r *bufio.Reader) {
	for {
		if !consume(r, ' ') && !consume(r, '\n') {
			return
		}
	}
}

// Parse a quoted string, which may contain escape sequences (e.g. \", \n).
// The second return is false if the parsing fails, which happens when:
// - the first character in the reader is not "
// - the input terminates before the ending "
// - the string contains invalid escape sequences
func scanQuoted(r *bufio.Reader) (string, bool) {
	// must start with "
	if !consume(r, '"') {
		return "", false
	}

	s := "\""

	escape := false
	var c rune

	for {
		_, err := fmt.Fscanf(r, "%c", &c)
		if err != nil {
			return s, false
		}

		if c == '\\' && !escape {
			// escape next character
			escape = true
			continue
		}

		s = s + string(c)

		if c == '"' && !escape {
			// Unquote handles the escape sequences other than \"
			s, err := strconv.Unquote(s)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v", err)
				return s, false
			}

			return s, true
		}

		escape = false
	}
}

// Parses a line with the format:
//
//	<id> [label=<name>] // <sendText> <sendInterval> <relayMode> <paused> <x> <y>
//
// Creates a node with such parameters, adds it to `net` and spawns its goroutine.
// Returns false if parsing fails.
func deserializeNode(
	r *bufio.Reader,
	net network,
	id nodeid,
	stopChan chan nodeid,
	reportChan chan sendreport,
) bool {
	var sendInterval int
	var relayMode relaymode
	var paused bool
	var x, y int

	_, err := fmt.Fscanf(r, "[label=")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error parsing [label=")
		return false
	}

	name, ok := scanQuoted(r)
	if !ok {
		fmt.Fprintln(os.Stderr, "error parsing <name>")
		return false
	}

	_, err = fmt.Fscanf(r, "] // ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error parsing ] //")
		return false
	}

	sendText, ok := scanQuoted(r)
	if !ok {
		fmt.Fprintln(os.Stderr, "error parsing <sendText>")
		return false
	}

	_, err = fmt.Fscanf(r, "%d %d %t %d %d",
		&sendInterval, &relayMode, &paused, &x, &y)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error parsing <sendInterval> <relayMode> <paused> <x> <y>")
		return false
	}

	net.spawnNodeWithID(id, x, y, stopChan, reportChan)

	if paused {
		net.togglePause(id)
	}

	net.setName(id, name)
	net.setSendText(id, sendText)
	net.setSendInterval(id, sendInterval)
	net.setRelayMode(id, relayMode)

	return true
}

// parses the endpoints of a channel and adds it to `endpoints`
func deserializeChan(r *bufio.Reader, chans []endpoints, src nodeid) ([]endpoints, bool) {
	var dst nodeid

	_, err := fmt.Fscanf(r, "-> %d", &dst)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error parsing -> <dst>")
		return chans, false
	}

	return append(chans, endpoints{src, dst}), true
}

// Deserializes a network stored in the format described in the README.org file.
//
// Returns the new network and the maximum node id it contains (so that
// nextNodeId from network.go can be updated).
//
// Also spawns the nodes in the new network. To avoid interferences with the old
// nodes, all running nodes must be stopped with stopAllAndWait before calling
// deserialize.
func deserialize(
	reader io.Reader,
	stopChan chan nodeid,
	reportChan chan sendreport,
) (network, nodeid) {
	r := bufio.NewReader(reader)

	var ok bool
	var maxid nodeid

	net := make(network)
	chans := make([]endpoints, 0)

	skipEmptyLines(r)

	_, err := fmt.Fscanf(r, "digraph network {\n")
	if err != nil {
		fmt.Fprintf(os.Stderr, "deserialization error: %v\nexpected 'digraph network {'\n", err)
		return nil, -1
	}

	for {
		skipEmptyLines(r)

		if consume(r, '}') {
			// found closing }
			break
		}

		// node format:
		// <id> [label=<name>] // <sendText> <sendInterval> <relayMode> <paused> <x> <y>
		// channel format:
		// <src> -> <dst>

		var id nodeid

		// both node and channels line start with an id, so we first
		// parse it, then call deserializeNode or deserializeChan
		// depending on whether the following character is '[' (as in
		// '[label=') or '-' (as in '->')

		_, err = fmt.Fscanf(r, "%d ", &id)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing <id>")
			return nil, -1
		}

		switch {
		case peek(r, '['):
			if !deserializeNode(r, net, id, stopChan, reportChan) {
				return nil, -1
			}

			maxid = max(maxid, id)

		case peek(r, '-'):
			// channels are not added to the network straight away,
			// as their endpoints may not already have been parsed;
			// for the moment we gather them in `chans`
			chans, ok = deserializeChan(r, chans, id)
			if !ok {
				return nil, -1
			}

		default:
			fmt.Fprintln(os.Stderr, "parse error: expected node or channel")
			return nil, -1
		}

	}

	// now we can add the channels we found in the file, since all nodes
	// have been created
	for _, c := range chans {
		net.addOrDelChan(c.src, c.dst)
	}

	return net, maxid
}
