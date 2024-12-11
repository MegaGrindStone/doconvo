package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// formFilePicker wraps a huh.FilePicker.
//
// We need this to override the KeyBinds method which determines what keys is shown
// in the help view.
type formFilePicker struct {
	*huh.FilePicker
	km huh.FilePickerKeyMap
}

func newFormFilePicker(fp *huh.FilePicker, km huh.FilePickerKeyMap) formFilePicker {
	return formFilePicker{
		FilePicker: fp,
		km:         km,
	}
}

func defaultList(title string, km keymap, shortHelps, fullHelps func() []key.Binding) list.Model {
	l := list.New([]list.Item{}, listDelegate(), 0, 0)
	l.Title = title
	l.Styles.Title = titleStyle
	// Remove horizontal padding from the title bar for consistency.
	l.Styles.TitleBar = lipgloss.NewStyle()
	l.DisableQuitKeybindings()

	l.AdditionalShortHelpKeys = shortHelps
	l.AdditionalFullHelpKeys = fullHelps

	l.KeyMap.Quit = km.quit
	l.KeyMap.ShowFullHelp = km.openHelp
	l.KeyMap.CloseFullHelp = km.closeHelp

	return l
}

func listDelegate() list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = listSelectedTitleStyle
	delegate.Styles.SelectedDesc = listDescSelectedStyle
	delegate.Styles.NormalTitle = listTitleStyle
	delegate.Styles.NormalDesc = listDescStyle

	return delegate
}

func logoView() string {
	return logoStyle.Render(logo)
}

func logoHeight() int {
	return lipgloss.Height(logoView())
}

func errView(width int, err error) string {
	return errorStyle.Render(fmt.Sprintf("Error: %s%s",
		err, strings.Repeat(" ", width)))
}

func errHeight(width int, err error) int {
	return lipgloss.Height(errView(width, err))
}

// logo is generated at https://patorjk.com/software/taag/#p=display&f=Slant&t=DOConvo
const logo = `
    ____  ____  ______                     
   / __ \/ __ \/ ____/___  ____ _   ______ 
  / / / / / / / /   / __ \/ __ \ | / / __ \
 / /_/ / /_/ / /___/ /_/ / / / / |/ / /_/ /
/_____/\____/\____/\____/_/ /_/|___/\____/ 
                                           
`

// These styles are based on the official Catppuccin palette:
// https://github.com/catppuccin/catppuccin#-palette
var (
	// General styles

	logoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#e64553", Dark: "#f38ba8"}). // Red
			Bold(true).
			PaddingBottom(1)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#e64553", Dark: "#f38ba8"}). // Red
			Bold(true).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#dc8a78", Dark: "#f2cdcd"}). // Rosewater
			PaddingLeft(2).
			PaddingRight(2).
			MarginBottom(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#eff1f5", Dark: "#cdd6f4"}). // Text color (Base)
			Background(lipgloss.AdaptiveColor{Light: "#e64553", Dark: "#d20f39"}). // Red (darker variant)
			Bold(true).
			Padding(0, 1)

	// List styles

	listSelectedTitleStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				Foreground(lipgloss.AdaptiveColor{Light: "#e64553", Dark: "#f38ba8"}).       // Red
				BorderForeground(lipgloss.AdaptiveColor{Light: "#dc8a78", Dark: "#f2cdcd"}). // Rosewater
				Padding(0, 0, 0, 1)

	listDescSelectedStyle = listSelectedTitleStyle.
				Foreground(lipgloss.AdaptiveColor{Light: "#9ca0b0", Dark: "#a6adc8"}) // Overlay0

	listTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#ea76cb", Dark: "#f5c2e7"}). // Pink
			Bold(true)

	listDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#9ca0b0", Dark: "#a6adc8"}). // Overlay0
			Italic(true)

	// Chat styles

	chatEntityStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#ea76cb", Dark: "#f5c2e7"}). // Pink
			Bold(true)

	chatContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#4c4f69", Dark: "#cdd6f4"}). // Text
				Padding(0, 4)

	chatTextareaStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.AdaptiveColor{Light: "#dc8a78", Dark: "#f2cdcd"}). // Rosewater
				Padding(1)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#7287fd", Dark: "#b4befe"}). // Lavender
			Bold(true).
			PaddingLeft(1).
			PaddingRight(1)
)

func (f formFilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	filePicker, cmd := f.FilePicker.Update(msg)
	if fp, ok := filePicker.(*huh.FilePicker); ok {
		f.FilePicker = fp
	}

	return f, cmd
}

func (f formFilePicker) KeyBinds() []key.Binding {
	f.km.Select.SetEnabled(true)
	f.km.Back.SetEnabled(true)
	return []key.Binding{f.km.Open, f.km.Back, f.km.Select, f.km.Close, f.km.Prev, f.km.Next}
}
