package main

import (
	"image"
	"log"
	"math"

	"image/color"

	"github.com/ebitenui/ebitenui"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// keeps most of the program state
type Game struct {
	ui          *ebitenui.UI
	toolbarRect image.Rectangle // to detect clicks on the toolbar

	xPan, yPan int // world origin in screen space

	wmx, wmy int // world mouse coordinates
	smx, smy int // screen mouse coordinates

	net        network         // info about the nodes in the network
	stopChan   chan nodeid     // used by nodes to signal termination
	reportChan chan sendreport // used by nodes to report every send (for coloring edges)

	connecting  bool   // true if currently drawing a new edge
	connectFrom nodeid // source of new edge

	selectedNode nodeid // current node for popup window
}

type tool int

const (
	TOOL_NODE tool = iota
	TOOL_EDGE
)

var currentTool tool

// Channels have an usage tracker that is used to compute the color of the
// channel's edge in the UI. Channels used more recently have a darker shade of
// gray. The usage is a float in range [0, 1], and it is reduced by
// CHAN_USAGE_DECAY on every update.
const CHAN_USAGE_DECAY = 0.02

func main() {
	var err error

	// fill some global variables with images for nodes, buttons etc.
	makeImages()

	face, err = loadFont(20)
	if err != nil {
		log.Fatal(err)
		return
	}

	game := Game{
		net:        make(network),
		stopChan:   make(chan nodeid, 32),
		reportChan: make(chan sendreport, 1024),
	}

	// toolbarRect is the area under the buttons at the top of the screen
	ui, toolbarRect := makeUI(&game)

	game.ui = &ui
	game.toolbarRect = toolbarRect

	ebiten.SetWindowSize(900, 500)
	ebiten.SetWindowTitle("Network Manager")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	// ebiten calls game's Update and Draw methods every update/draw tick,
	// 60 times per second
	if err = ebiten.RunGame(&game); err != nil {
		log.Fatal(err)
	}
}

// required for window resizing
func (g *Game) Layout(outsideWidth int, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

// returns the node at the current mouse position
// the second return value is false if no node was found
func (g *Game) nodeAtMouse() (nodeid, bool) {
	// look for a node which is at (manhattan) distance r from the mouse
	// should be nodeSize/2 but we double it to help with misclicks
	r := nodeSize

	// scan all nodes and return the first that matches
	for k, n := range g.net {
		if abs(g.wmx-n.x) <= r && abs(g.wmy-n.y) <= r {
			return k, true
		}
	}

	return -1, false
}

// returns position of node in screen space,
// i.e. offset by the values of xPan and yPan
func (g *Game) nodeScreenPos(id nodeid) (x, y int) {
	x = g.net[id].x + g.xPan
	y = g.net[id].y + g.yPan
	return
}

// Update is called by ebiten 60 times per second,
// and implements the logic of the main goroutine.
func (g *Game) Update() error {
	// For every channel, we keep a usage tracker which is used to compute
	// the color of the channel's edge in the visualization. Every update,
	// we apply decay to all the trackers, reducing them by CHAN_USAGE_DECAY.
	for i := range g.net {
		n := g.net[i]

		for j := range n.outs {
			u := n.outs[j].usage
			n.outs[j].usage = max(0, u-CHAN_USAGE_DECAY)
		}

		// Go forbids direct modification of values in maps,
		// so we often need to copy the node struct into
		// a variable (n), modify the copy, and then write it
		// back into the map g.net
		g.net[i] = n
	}

loop1: // handle messages on reportChan
	for {
		select {
		case r := <-g.reportChan:
			// r is a struct of type sendreport
			// it contains a pair of node IDs, src and dst
			// it means that src has sent a message to dst

			n := g.net[r.src]

			// find the channel from src to dst and set its usage
			// tracker to 1
			for i := range n.outs {
				if n.outs[i].dst == r.dst {
					n.outs[i].usage = 1
				}
			}

			g.net[r.src] = n

		default:
			break loop1
		}
	}

loop2: // handle messages on reportChan
	for {
		select {
		case i := <-g.stopChan:
			// node i quit, we can delete it
			delete(g.net, i)

		default:
			break loop2
		}
	}

	// call ebitenui update function
	g.ui.Update()
	if g.ui.HasFocus() {
		// click on popup window
		return nil
	}

	mx, my := ebiten.CursorPosition()

	if g.toolbarRect.At(mx, my) == color.Opaque {
		// click on toolbar
		return nil
	}

	// g.smx, g.smy store the values of mx, my in the last update
	// we compute the mouse movement since the last update
	dx, dy := mx-g.smx, my-g.smy

	// then we update g.smx, g.smy with the new position
	g.smx, g.smy = mx, my

	// right mouse button is for panning: if it is held down, move the
	// coordinates of the world origin relative to the screen by dx, dy
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		g.xPan += dx
		g.yPan += dy
	}

	// store the coordinates of the mouse in world space
	g.wmx, g.wmy = mx-g.xPan, my-g.yPan

	// the rest of the function handles left clicks

	if !inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		// return if no left click
		return nil
	}

	switch currentTool {
	case TOOL_NODE:

		// with the node tool, the user can:
		// - click on a node to show its control panel
		// - click on an empty point to create a new node

		if id, ok := g.nodeAtMouse(); ok {
			showNodeCtlWindow(g, id)
		} else {
			// adds a node to g.net and spawns its goroutine
			g.net.spawnNode(g.wmx, g.wmy, g.stopChan, g.reportChan)
		}

	case TOOL_EDGE:
		// with the edge tool, the user can click on a node, then:
		// - click on a non-connected node to create a new channel
		// - click on an already connected node to remove the channel

		n, ok := g.nodeAtMouse()

		switch {
		case !ok:
			// on click on an empty point, cancel the operation
			g.connecting = false

		case g.connecting:

			// g.connecting is true, so the user just clicked on the
			// second node; add/delete the channel between the two
			// nodes if they are not the same node

			if g.connectFrom != n {
				g.net.addOrDelChan(g.connectFrom, n)
			}

			g.connecting = false

		default:

			// g.connecting is false, so this is the first node of
			// the connection. Record it in g.connectFrom and
			// set g.connecting.

			g.connectFrom = n
			g.connecting = true
		}
	}

	return nil
}

// called by ebiten to draw the screen
func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.White)

	// if the user is connecting a node with the edge tool, draw a line
	// from the source to the mouse pointer
	if g.connecting {
		id := g.connectFrom
		x, y := g.nodeScreenPos(id)

		vector.StrokeLine(screen,
			float32(x), float32(y),
			float32(g.smx), float32(g.smy),
			2, color.Black, true)
	}

	// draw a line between the endpoints of each channel
	for k, n := range g.net {
		for j := range n.outs {
			o := n.outs[j]

			x0, y0 := g.nodeScreenPos(k)
			x1, y1 := g.nodeScreenPos(o.dst)

			// shade of gray depends on usage
			v := uint8(0xEE * (1 - o.usage))

			// channel line
			vector.StrokeLine(screen,
				float32(x0), float32(y0),
				float32(x1), float32(y1),
				2, color.Gray{Y: v}, true)

			// channels have a direction indicator, which is
			// a triangle drawn on the line; here we compute the
			// rotation for the triangle and draw it

			a := math.Atan2(float64(y1-y0), float64(x1-x0))
			dx, dy := float64(x1-x0), float64(y1-y0)
			d := math.Sqrt(dx*dx + dy*dy)

			drawTriangle(screen,
				x0+int(dx/d*2*nodeSize), // center x
				y0+int(dy/d*2*nodeSize), // center y
				nodeSize*3/4,            // radius
				a,                       // rotation angle
				v,                       // color
			)
		}
	}

	// draw the nodes (on top of the channels)

	for id := range g.net {
		x, y := g.nodeScreenPos(id)

		// coordinates of the top left corner of the node image
		x = x - nodeSize/2
		y = y - nodeSize/2

		op := ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(x), float64(y))

		// draw a different image (gray instead of black) for paused nodes
		if g.net[id].paused {
			screen.DrawImage(pausedNodeImage, &op)
		} else {
			screen.DrawImage(nodeImage, &op)
		}
	}

	// finally, call ebitenui to draw the UI
	g.ui.Draw(screen)
}
