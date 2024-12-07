package main

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/huh"
)

type keymap struct {
	listKeymap

	textAreaKeymap textarea.KeyMap
	viewportKeymap viewport.KeyMap
	formKeymap     *huh.KeyMap

	submit    key.Binding
	openHelp  key.Binding
	closeHelp key.Binding
	quit      key.Binding
	escape    key.Binding
	option    key.Binding

	viewState viewState
}

type listKeymap struct {
	new    key.Binding
	delete key.Binding
	pick   key.Binding // Can't use select because it's a reserved word
}

func newKeymap() keymap {
	return keymap{
		listKeymap:     newListKeymap(),
		textAreaKeymap: newTextAreaKeymap(),
		viewportKeymap: newViewportKeymap(),
		formKeymap:     newFormKeymap(),
		submit: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "submit"),
		),
		openHelp: key.NewBinding(
			key.WithKeys("ctrl+h"),
			key.WithHelp("ctrl+h", "more"),
		),
		closeHelp: key.NewBinding(
			key.WithKeys("ctrl+h"),
			key.WithHelp("ctrl+h", "close help"),
		),
		quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		option: key.NewBinding(
			key.WithKeys("ctrl+o"),
			key.WithHelp("ctrl+o", "options"),
		),
		viewState: viewStateSessions,
	}
}

func newListKeymap() listKeymap {
	return listKeymap{
		new: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new"),
		),
		delete: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "delete"),
		),
		pick: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
	}
}

func newViewportKeymap() viewport.KeyMap {
	km := viewport.DefaultKeyMap()

	km.HalfPageDown.SetEnabled(false)
	km.HalfPageUp.SetEnabled(false)

	km.Up.SetKeys("ctrl+p")
	km.Up.SetHelp("ctrl+p", "chatbox up")

	km.Down.SetKeys("ctrl+n")
	km.Down.SetHelp("ctrl+n", "chatbox down")

	km.PageUp.SetKeys("pgup")
	km.PageUp.SetHelp("pgup", "chatbox page up")

	km.PageDown.SetKeys("pgdn")
	km.PageDown.SetHelp("pgdn", "chatbox page down")

	return km
}

func newTextAreaKeymap() textarea.KeyMap {
	km := textarea.DefaultKeyMap

	km.LineNext.SetKeys("down")
	km.LineNext.SetHelp("down", "next line")

	km.LinePrevious.SetKeys("up")
	km.LinePrevious.SetHelp("up", "previous line")

	return km
}

func newFormKeymap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()

	km.FilePicker.Open.SetHelp("→/l", "open")
	km.FilePicker.Back.SetHelp("←/h", "back")

	return km
}

func (k keymap) FullHelp() [][]key.Binding {
	if k.viewState == viewStateDocumentScan {
		return [][]key.Binding{
			{k.viewportKeymap.Up, k.viewportKeymap.Down, k.viewportKeymap.PageUp, k.viewportKeymap.PageDown, k.escape},
			{k.quit, k.closeHelp},
		}
	}
	return [][]key.Binding{
		{k.viewportKeymap.Up, k.viewportKeymap.Down, k.viewportKeymap.PageUp, k.viewportKeymap.PageDown, k.escape},
		{k.textAreaKeymap.InsertNewline, k.submit, k.quit, k.closeHelp},
	}
}

func (k keymap) ShortHelp() []key.Binding {
	if k.viewState == viewStateDocumentScan {
		return []key.Binding{k.escape, k.viewportKeymap.Up, k.viewportKeymap.Down, k.openHelp}
	}
	return []key.Binding{k.textAreaKeymap.InsertNewline, k.submit, k.quit, k.openHelp}
}
