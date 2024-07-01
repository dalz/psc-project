// This file contains functions to build and display the UI elements,
// mainly the toolbar at the top of the window and the node control panel.
// It contains very little logic, and is not further commented.

package main

import (
	go_image "image"
	"image/color"

	"strconv"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/golang/freetype/truetype"
	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"os"
)

var buttonImage *widget.ButtonImage

const nodeSize = 10

var nodeImage *ebiten.Image
var pausedNodeImage *ebiten.Image

var face font.Face

var pathSelectWindow *widget.Window
var pathSelectHandler func(string)
var nodeCtlWindow *widget.Window
var nameInput *widget.TextInput
var sendTextInput *widget.TextInput
var sendIntervalInput *widget.TextInput
var relayModeRadioGroup *widget.RadioGroup
var roundRobinBtn *widget.Button
var multicastBtn *widget.Button
var pauseBtnLabel *string
var errPopUpWindow *widget.Window
var errPopUpText *widget.Text

func NO_VALIDATOR(_ string) (bool, *string) {
	return true, nil
}

func loadButtonImage() *widget.ButtonImage {
	idle := image.NewNineSliceColor(color.Gray{Y: 150})
	hover := image.NewNineSliceColor(color.Gray{Y: 100})
	pressed := image.NewNineSliceColor(color.Gray{Y: 50})

	return &widget.ButtonImage{
		Idle:    idle,
		Hover:   hover,
		Pressed: pressed,
	}
}

func loadFont(size float64) (font.Face, error) {
	ttfFont, err := truetype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}

	return truetype.NewFace(ttfFont, &truetype.Options{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	}), nil
}

func addButton(
	container *widget.Container,
	text string,
	handler widget.ButtonClickedHandlerFunc,
) *widget.Button {
	btn := widget.NewButton(
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),

		widget.ButtonOpts.Image(buttonImage),

		widget.ButtonOpts.Text(text, face, &widget.ButtonTextColor{
			Idle: color.White,
		}),

		widget.ButtonOpts.TextPadding(widget.Insets{
			Top: 5, Left: 5, Right: 5, Bottom: 5}),

		widget.ButtonOpts.ClickedHandler(handler),
	)

	container.AddChild(btn)

	return btn
}

func addToolBtn(toolbar *widget.Container, text string, tool tool) *widget.Button {
	return addButton(toolbar, text, func(args *widget.ButtonClickedEventArgs) {
		currentTool = tool
	})
}

func makeToolbarWindow(g *Game) (*widget.Window, go_image.Rectangle) {
	toolbar := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(
			image.NewNineSliceColor(color.NRGBA{0x13, 0x1a, 0x22, 0xff})),

		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			// widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(30)),
			// widget.RowLayoutOpts.Spacing(20),
		)),
	)

	nodeToolBtn := addToolBtn(toolbar, "node", TOOL_NODE)
	edgeToolBtn := addToolBtn(toolbar, "edge", TOOL_EDGE)

	widget.NewRadioGroup(
		widget.RadioGroupOpts.Elements(nodeToolBtn, edgeToolBtn),
		widget.RadioGroupOpts.InitialElement(nodeToolBtn),
	)

	addButton(toolbar, "save", func(args *widget.ButtonClickedEventArgs) {
		promptPath(g, func(p string) {
			f, err := os.Create(p)
			if err != nil {
				errPopUp(g, "Couldn't create file")
				return
			}

			g.net.serialize(f)

			f.Close()
		})
	})

	addButton(toolbar, "load", func(args *widget.ButtonClickedEventArgs) {
		promptPath(g, func(p string) {
			f, err := os.Open(p)
			if err != nil {
				errPopUp(g, "Couldn't open file")
				return
			}

			g.net.stopAllAndWait(g.stopChan)

			net, maxid := deserialize(f, g.stopChan, g.reportChan)
			if net == nil {
				errPopUp(g, "Error during parsing")
				return
			}

			g.net = net
			nextNodeId = int(maxid) + 1

			f.Close()
		})
	})

	addButton(toolbar, "clear", func(args *widget.ButtonClickedEventArgs) {
		g.net.stopAllAndWait(g.stopChan)
		nextNodeId = 0
	})

	w := widget.NewWindow(widget.WindowOpts.Contents(toolbar))

	x, y := w.Contents.PreferredSize()
	r := go_image.Rect(0, 0, x, y)
	r = r.Add(go_image.Point{10, 10})
	w.SetLocation(r)

	return w, r
}

func makeErrWindow() {
	c := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(
			image.NewNineSliceColor(color.RGBA{0x66, 0x22, 0x22, 0xff})),

		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(10)),
		)),
	)

	errPopUpText = widget.NewText(
		widget.TextOpts.Text("                                   ", face, color.White),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
	)

	c.AddChild(errPopUpText)

	errPopUpWindow = widget.NewWindow(
		widget.WindowOpts.Contents(c),
		widget.WindowOpts.CloseMode(widget.CLICK),
	)

	x, y := errPopUpWindow.Contents.PreferredSize()
	r := go_image.Rect(0, 0, x, y)
	r = r.Add(go_image.Point{50, 100})
	errPopUpWindow.SetLocation(r)
}

func errPopUp(g *Game, text string) {
	errPopUpText.Label = text
	g.ui.AddWindow(errPopUpWindow)
}

func addRelayModeBtn(g *Game, container *widget.Container, text string, mode relaymode) *widget.Button {
	return addButton(container, text, func(args *widget.ButtonClickedEventArgs) {
		g.net.setRelayMode(g.selectedNode, mode)
	})
}

func addTextInput(
	container *widget.Container,
	label string,
	validator widget.TextInputValidationFunc,
	handler widget.TextInputChangedHandlerFunc,
) *widget.TextInput {
	ti := widget.NewTextInput(
		widget.TextInputOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),

			widget.WidgetOpts.MinSize(200, 0),
		),

		widget.TextInputOpts.Face(face),

		widget.TextInputOpts.Color(&widget.TextInputColor{
			Idle:          color.NRGBA{254, 255, 255, 255},
			Disabled:      color.NRGBA{R: 200, G: 200, B: 200, A: 255},
			Caret:         color.NRGBA{254, 255, 255, 255},
			DisabledCaret: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
		}),

		widget.TextInputOpts.Padding(widget.NewInsetsSimple(5)),

		widget.TextInputOpts.CaretOpts(widget.CaretOpts.Size(face, 2)),

		widget.TextInputOpts.Validation(validator),
		widget.TextInputOpts.SubmitHandler(handler),
	)

	lbl := widget.NewText(
		widget.TextOpts.Text(label+": ", face, color.White),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
	)

	rc := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			// widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(30)),
			// widget.RowLayoutOpts.Spacing(20),
		)),
	)

	rc.AddChild(lbl)
	rc.AddChild(ti)
	container.AddChild(rc)

	return ti
}

func makeNodeCtlWindow(g *Game) {
	container := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(
			image.NewNineSliceColor(color.Black)),

		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(5)),
			widget.RowLayoutOpts.Spacing(10),
		)),
	)

	nameInput = addTextInput(container, "Name",
		NO_VALIDATOR,

		func(args *widget.TextInputChangedEventArgs) {
			g.net.setName(g.selectedNode, args.InputText)
		})

	sendTextInput = addTextInput(container, "Send text",
		NO_VALIDATOR,

		func(args *widget.TextInputChangedEventArgs) {
			g.net.setSendText(g.selectedNode, args.InputText)
		})

	sendIntervalInput = addTextInput(container, "Send interval (ms)",
		func(input string) (bool, *string) {
			// _, err := strconv.ParseFloat(input, 64)
			_, err := strconv.Atoi(input)
			return err == nil, nil
		},

		func(args *widget.TextInputChangedEventArgs) {
			ms, _ := strconv.Atoi(args.InputText)
			g.net.setSendInterval(g.selectedNode, ms)
		})

	relayModeLabel := widget.NewText(
		widget.TextOpts.Text("Relay mode: ", face, color.White),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
	)

	relayModeRow := widget.NewContainer(
		// widget.ContainerOpts.BackgroundImage(
		// 	image.NewNineSliceColor(color.NRGBA{0x13, 0x1a, 0x22, 0xff})),

		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			// widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(30)),
			// widget.RowLayoutOpts.Spacing(20),
		)),
	)

	relayModeRow.AddChild(relayModeLabel)

	roundRobinBtn = addRelayModeBtn(g, relayModeRow, "round-robin", ROUND_ROBIN)
	multicastBtn = addRelayModeBtn(g, relayModeRow, "multicast", MULTICAST)
	multicastBtn = addRelayModeBtn(g, relayModeRow, "discard", DISCARD)

	relayModeRadioGroup = widget.NewRadioGroup(
		widget.RadioGroupOpts.Elements(roundRobinBtn, multicastBtn),
	)

	container.AddChild(relayModeRow)

	buttonsRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			// widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(30)),
			widget.RowLayoutOpts.Spacing(5),
		)),
	)

	pauseBtnLabel = &addButton(buttonsRow, "pause", func(args *widget.ButtonClickedEventArgs) {
		if g.net.togglePause(g.selectedNode) {
			*pauseBtnLabel = "resume"
		} else {
			*pauseBtnLabel = " pause "
		}
	}).Text().Label

	addButton(buttonsRow, "delete", func(args *widget.ButtonClickedEventArgs) {
		g.net.stop(g.selectedNode)
		nodeCtlWindow.Close()
	})

	addButton(buttonsRow, "cancel", func(args *widget.ButtonClickedEventArgs) {
		nodeCtlWindow.Close()
	})

	addButton(buttonsRow, "apply", func(args *widget.ButtonClickedEventArgs) {
		nameInput.Submit()
		sendTextInput.Submit()
		sendIntervalInput.Submit()
		nodeCtlWindow.Close()
	})

	container.AddChild(buttonsRow)

	nodeCtlWindow = widget.NewWindow(
		widget.WindowOpts.Contents(container),
		widget.WindowOpts.CloseMode(widget.NONE),
		widget.WindowOpts.Modal(),
	)
}

func makePathSelectWindow() {
	container := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(
			image.NewNineSliceColor(color.Black)),

		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(5)),
			widget.RowLayoutOpts.Spacing(10),
		)),
	)

	pathInput := addTextInput(container, "Path", NO_VALIDATOR,
		func(args *widget.TextInputChangedEventArgs) {
			pathSelectHandler(args.InputText)
			pathSelectWindow.Close()
		})

	buttonsRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			// widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(30)),
			widget.RowLayoutOpts.Spacing(5),
		)),
	)

	addButton(buttonsRow, "cancel", func(args *widget.ButtonClickedEventArgs) {
		pathSelectWindow.Close()
	})

	addButton(buttonsRow, "confirm", func(args *widget.ButtonClickedEventArgs) {
		pathInput.Submit()
		pathSelectWindow.Close()
	})

	container.AddChild(buttonsRow)

	pathSelectWindow = widget.NewWindow(
		widget.WindowOpts.Contents(container),
		widget.WindowOpts.CloseMode(widget.CLICK_OUT),
		widget.WindowOpts.Modal(),
	)

	x, y := pathSelectWindow.Contents.PreferredSize()
	r := go_image.Rect(0, 0, x, y)
	r = r.Add(go_image.Point{50, 100})
	pathSelectWindow.SetLocation(r)
}

func showNodeCtlWindow(g *Game, id nodeid) {
	g.selectedNode = id

	nameInput.SetText(g.net[g.selectedNode].name)
	sendTextInput.SetText(g.net[g.selectedNode].sendText)
	sendIntervalInput.SetText(
		strconv.FormatInt(
			g.net[g.selectedNode].sendInterval.Milliseconds(),
			10))

	switch g.net[id].relayMode {
	case ROUND_ROBIN:
		relayModeRadioGroup.SetActive(roundRobinBtn)
	case MULTICAST:
		relayModeRadioGroup.SetActive(multicastBtn)
	}

	if g.net[id].paused {
		*pauseBtnLabel = "resume"
	} else {
		*pauseBtnLabel = " pause "
	}

	rw, rh := nodeCtlWindow.Contents.PreferredSize()
	r := go_image.Rect(0, 0, rw, rh)
	r = r.Add(go_image.Point{g.smx, g.smy})
	nodeCtlWindow.SetLocation(r)

	g.ui.AddWindow(nodeCtlWindow)
}

func promptPath(g *Game, handler func(string)) {
	pathSelectHandler = handler
	g.ui.AddWindow(pathSelectWindow)
}

func makeUI(g *Game) (ebitenui.UI, go_image.Rectangle) {
	ui := ebitenui.UI{
		Container: widget.NewContainer(),
	}

	toolbarWindow, toolbarRect := makeToolbarWindow(g)

	makeNodeCtlWindow(g)
	makePathSelectWindow()
	makeErrWindow()

	ui.AddWindow(toolbarWindow)

	return ui, toolbarRect
}

func makeImages() {
	buttonImage = loadButtonImage()

	nodeImage = ebiten.NewImage(nodeSize, nodeSize)
	nodeImage.Fill(color.Black)

	pausedNodeImage = ebiten.NewImage(nodeSize, nodeSize)
	pausedNodeImage.Fill(color.Gray{Y: 150})
}
