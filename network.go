package main

import (
	"log"
	"slices"
	"strconv"
	"time"
)

// type of the channels where instructions are sent to nodes from the main goroutine
type ctlchan chan ctlmsg

// type of the channels between nodes
type datachan chan string

// buffer size for datachans
const CHAN_BUF_SIZE = 128

// Information kept by main about each channel.
// It corresponds to nodeout (see below), which stores the information kept by
// the source nodes about the same channels.
type chaninfo struct {
	dst nodeid

	// set to 1 every time the channel is used, decays gradually,
	// used for coloring the channel in the UI
	usage float64
}

// messages on ctlchans are composed of a tag, which specifies the action the
// receiving node should perform, and a payload which is action-dependent
type ctlmsg struct {
	action  ctlact
	payload interface{}
}

type ctlact int

const (
	// set node display name
	SET_NAME ctlact = iota

	// add/remove a channel
	ADD_DEST
	DEL_DEST

	// how received messages should be retrasmitted:
	// round-robin, multicast, no relay
	SET_RELAY_MODE

	// for messages generated by this node, what should be the text
	// and how frequently should they be sent out
	SET_SEND_TEXT
	SET_SEND_INTERVAL

	TOGGLE_PAUSE
	QUIT
)

// Information kept by the nodes about their outgoing channels. Other than the
// channel itself, we need the id of the destination for the purpose of
// printing and reporting.
// It corresponds to chaninfo, which stores the information kept by the main
// goroutine about the same channels.
type nodeout struct {
	ch  datachan
	dst nodeid
}

// struct sent on the reportChan after each send,
// it just contains source and destination nodes
type sendreport struct {
	src, dst nodeid
}

// how received messages should be retrasmitted
type relaymode int

const (
	ROUND_ROBIN relaymode = iota
	MULTICAST
	DISCARD
)

// each node has a fixed unique numeric id assigned at creation
type nodeid int

// information kept by the main goroutine about each node
type node struct {
	id   nodeid
	name string // display name, used in the UI and text output

	// each node has its own:
	// - control channel, for receiving commands from the main goroutine
	// - input channel, for receiving data from other nodes. Main needs to
	//   know it to create new connections.
	ctl ctlchan
	in  datachan

	// for each output channel of this node, a chaninfo struct holds its
	// destination and recent usage count
	outs []chaninfo

	// The text, interval and relay mode (round-robin, multicast, discard)
	// of this node. Main needs to know those to show them in the node
	// control panel.
	sendText     string
	sendInterval time.Duration
	relayMode    relaymode

	paused bool

	// coordinates of the node in world space, purely for the visualization
	x, y int
}

// main gathers the nodes in a map indexed by node IDs
type network map[nodeid]node

// add a new node to the network, and spawn its goroutine
// the ID is passed as a parameter (needed when loading a network from a file)
func (net network) spawnNodeWithID(
	id nodeid,
	x, y int,
	stopChan chan nodeid,
	reportChan chan sendreport,
) {
	// create node with some defaults
	n := node{
		id:   id,
		name: "node",

		ctl: make(ctlchan, 16),
		in:  make(datachan, CHAN_BUF_SIZE),

		sendText: "from " + strconv.Itoa(int(id)),

		relayMode: ROUND_ROBIN,

		x: x,
		y: y,
	}

	net[id] = n

	// spawn node goroutine
	go nodeMain(n, stopChan, reportChan)
}

var nextNodeId = 0

// same as spawnNodeWithID, but the value of nextNodeId is used as ID
func (net network) spawnNode(
	x, y int,
	stopChan chan nodeid,
	reportChan chan sendreport,
) {
	net.spawnNodeWithID(nodeid(nextNodeId), x, y, stopChan, reportChan)
	nextNodeId += 1
}

// utility function to send a control message to a node
func (net network) sendCtl(dst nodeid, action ctlact, payload interface{}) {
	net[dst].ctl <- ctlmsg{action: action, payload: payload}
}

// now follow some functions that change the parameters of a running node by
// sending a control message, and also update the corresponding parameter in the
// network map

func (net network) setName(id nodeid, name string) {
	n := net[id]
	n.name = name
	net[id] = n

	net.sendCtl(id, SET_NAME, name)
}

// create a channel between i and j if there is none yet, delete it otherwise
func (net network) addOrDelChan(i nodeid, j nodeid) {
	for k, c := range net[i].outs {
		if c.dst != j {
			continue
		}

		// found and existing channel between i and j, delete it

		// delete from net
		n := net[i]
		n.outs = slices.Delete(n.outs, k, k+1)
		net[i] = n

		// tell the node to delete it too
		net.sendCtl(i, DEL_DEST, j)

		return
	}

	// new channel, create it

	// add in net
	n := net[i]
	n.outs = append(n.outs, chaninfo{dst: j})
	net[i] = n

	// tell the node to add it too
	net.sendCtl(i, ADD_DEST, nodeout{ch: net[j].in, dst: j})
}

func (net network) setRelayMode(id nodeid, mode relaymode) {
	n := net[id]
	n.relayMode = mode
	net[id] = n

	net.sendCtl(id, SET_RELAY_MODE, mode)
}

func (net network) setSendText(id nodeid, text string) {
	n := net[id]
	n.sendText = text
	net[id] = n

	net.sendCtl(id, SET_SEND_TEXT, text)
}

func (net network) setSendInterval(id nodeid, ms int) {
	d := time.Duration(ms * 1_000_000)

	n := net[id]
	n.sendInterval = d
	net[id] = n

	net.sendCtl(id, SET_SEND_INTERVAL, d)
}

func (net network) togglePause(id nodeid) bool {
	n := net[id]
	n.paused = !n.paused
	net[id] = n

	net.sendCtl(id, TOGGLE_PAUSE, nil)

	return n.paused
}

func (net network) stop(id nodeid) {
	net.sendCtl(id, QUIT, nil)
}

// send a QUIT message to all nodes and wait for their termination
func (net network) stopAllAndWait(stopChan chan nodeid) {
	log.Printf("[manager] stopping all nodes")

	for id := range net {
		net.stop(id)
	}

	// delete all nodes, not necessarily in the order we receive their
	// quit notifications; we just need to make sure that they all
	// terminated before proceeding
	for i := range net {
		_ = <-stopChan
		delete(net, i)
	}
}

// the code executed by nodes in their goroutines is entirely contained in this function
func nodeMain(params node, stopChan chan nodeid, reportChan chan sendreport) {
	id, name := params.id, params.name
	sendText, sendInterval := params.sendText, params.sendInterval
	relayMode := params.relayMode
	in, ctl := params.in, params.ctl

	logMsg := func(s string, args ...any) {
		args = append([]any{name, id}, args...)
		log.Printf("[%s %v] "+s+"\n", args...)
	}

	defer logMsg("quit")

	// alert main when this node stops
	defer func() { stopChan <- id }()

	logMsg("start")

	// output channels for this node
	var outs []nodeout

	// timer that fires every sendInterval
	sendTicker := time.NewTicker(2 << 30)
	if sendInterval > 0 {
		sendTicker.Reset(sendInterval)
	} else {
		sendTicker.Stop()
	}

	// for round-robin
	nextOut := 0

	// input channel when running, nil when paused
	// we set it to nil when paused so that the select statement below
	// ignores input messages
	inOrNil := in

loop: // repeat until main sends a QUIT message
	for {
		select {
		case c := <-ctl:
			// control message from main received, change the
			// appropriate parameters

			switch c.action {
			case SET_NAME:
				logMsg("change name to %v", c.payload)

				name = c.payload.(string)

			case ADD_DEST:
				o := c.payload.(nodeout)

				logMsg("add output channel to node %v", o.dst)

				outs = append(outs, o)

			case DEL_DEST:
				dst := c.payload.(nodeid)

				logMsg("delete output channel to node %v", dst)

				outs = slices.DeleteFunc(
					outs,
					func(o nodeout) bool {
						return o.dst == dst
					},
				)

			case SET_RELAY_MODE:
				logMsg("change relay mode to %v", c.payload)

				relayMode = c.payload.(relaymode)

			case SET_SEND_TEXT:
				logMsg("change send text to node %v", c.payload)

				sendText = c.payload.(string)

			case SET_SEND_INTERVAL:
				sendInterval = c.payload.(time.Duration)

				logMsg("change send interval to %v ms",
					float64(sendInterval)/1_000_000)

				if sendInterval > 0 {
					sendTicker.Reset(sendInterval)
				} else {
					sendTicker.Stop()
				}

			case TOGGLE_PAUSE:
				if inOrNil == nil {
					// currently paused, resume by setting
					// inOrNil to the input channel in
					inOrNil = in

					// also resume generating messages
					if sendInterval > 0 {
						sendTicker.Reset(sendInterval)
					}

				} else {
					// currently running, pause
					inOrNil = nil     // ignore incoming messages
					sendTicker.Stop() // stop generating messages
				}

			case QUIT:
				break loop
			}

		case s := <-inOrNil:
			// incoming message from another node
			// note that when paused inOrNil is nil, so we don't
			// handle incoming messages

			logMsg("received message \"%s\"", s)

			// we have to relay the message according to the relayMode

			if len(outs) == 0 {
				// nothing to do, we have no output channel
				break
			}

			switch relayMode {
			case ROUND_ROBIN:
				// forward to one output only (nextOut)

				o := outs[nextOut]

				logMsg("relay to node %v (round-robin)", o.dst)

				// relay message
				o.ch <- s

				// increment cyclic counter
				nextOut = (nextOut + 1) % len(outs)

				// notify main of the send (for channel usage tracking)
				reportChan <- sendreport{src: id, dst: o.dst}

			case MULTICAST:
				// forward to all outputs

				for _, o := range outs {
					logMsg("relay to node %v (multicast)", o.dst)

					o.ch <- s

					reportChan <- sendreport{src: id, dst: o.dst}
				}

			case DISCARD:
				// do nothing
			}

		case <-sendTicker.C:
			// a message is sent on sendTicker.C every time the
			// timer fires, i.e. every sendInterval

			// we generate a new message with text sendText and send
			// it to all the outputs; message generation is always
			// multicast irrespectively of the relayMode

			for _, o := range outs {
				logMsg("send to node %v (multicast)", o.dst)

				o.ch <- sendText

				reportChan <- sendreport{src: id, dst: o.dst}
			}
		}
	}
}
