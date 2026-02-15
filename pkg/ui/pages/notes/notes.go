package notes

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/soft-serve/pkg/ui/common"
	"github.com/charmbracelet/soft-serve/pkg/ui/components/code"
	"github.com/charmbracelet/soft-serve/pkg/ui/components/selector"
	"github.com/dustin/go-humanize"
)

type notesView int

const (
	notesViewFiles notesView = iota
	notesViewContent
)

var (
	errNoFileSelected = errors.New("no file selected")
	errBinaryFile     = errors.New("binary file")
)

var (
	lineNo = key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("l", "toggle line numbers"),
	)
)

// FileItemsMsg is a message that contains a list of files.
type FileItemsMsg []selector.IdentifiableItem

// FileContentMsg is a message that contains the content of a file.
type FileContentMsg struct {
	content string
	ext     string
}

// NotesFileItem is a list item for a file in the notes directory.
type NotesFileItem struct {
	name  string
	path  string
	isDir bool
	size  int64
	mode  fs.FileMode
}

// ID returns the ID of the file item.
func (i NotesFileItem) ID() string {
	return i.name
}

// Title returns the title of the file item.
func (i NotesFileItem) Title() string {
	return common.UnquoteFilename(i.name)
}

// Description returns the description of the file item.
func (i NotesFileItem) Description() string {
	return ""
}

// Mode returns the mode of the file item.
func (i NotesFileItem) Mode() fs.FileMode {
	return i.mode
}

// FilterValue implements list.Item.
func (i NotesFileItem) FilterValue() string { return i.Title() }

// NotesFileItemDelegate is the delegate for the file item list.
type NotesFileItemDelegate struct {
	common *common.Common
}

// Height returns the height of the file item list. Implements list.ItemDelegate.
func (d NotesFileItemDelegate) Height() int { return 1 }

// Spacing returns the spacing of the file item list. Implements list.ItemDelegate.
func (d NotesFileItemDelegate) Spacing() int { return 0 }

// Update implements list.ItemDelegate.
func (d NotesFileItemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	item, ok := m.SelectedItem().(NotesFileItem)
	if !ok {
		return nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, d.common.KeyMap.Copy):
			return copyCmd(item.name, fmt.Sprintf("File name %q copied to clipboard", item.name))
		}
	}
	return nil
}

// Render implements list.ItemDelegate.
func (d NotesFileItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(NotesFileItem)
	if !ok {
		return
	}

	s := d.common.Styles.Tree

	name := i.Title()
	size := humanize.Bytes(uint64(i.size)) //nolint:gosec
	size = strings.ReplaceAll(size, " ", "")
	sizeLen := lipgloss.Width(size)
	if i.isDir {
		size = strings.Repeat(" ", sizeLen)
		if index == m.Index() {
			name = s.Active.FileDir.Render(name)
		} else {
			name = s.Normal.FileDir.Render(name)
		}
	}
	var nameStyle, sizeStyle, modeStyle lipgloss.Style
	mode := i.Mode()
	if index == m.Index() {
		nameStyle = s.Active.FileName
		sizeStyle = s.Active.FileSize
		modeStyle = s.Active.FileMode
		fmt.Fprint(w, s.Selector.Render(">")) //nolint:errcheck
	} else {
		nameStyle = s.Normal.FileName
		sizeStyle = s.Normal.FileSize
		modeStyle = s.Normal.FileMode
		fmt.Fprint(w, s.Selector.Render(" ")) //nolint:errcheck
	}
	sizeStyle = sizeStyle.
		Width(8).
		Align(lipgloss.Right).
		MarginLeft(1)
	leftMargin := s.Selector.GetMarginLeft() +
		s.Selector.GetWidth() +
		s.Normal.FileMode.GetMarginLeft() +
		s.Normal.FileMode.GetWidth() +
		nameStyle.GetMarginLeft() +
		sizeStyle.GetHorizontalFrameSize()
	name = common.TruncateString(name, m.Width()-leftMargin)
	name = nameStyle.Render(name)
	size = sizeStyle.Render(size)
	modeStr := modeStyle.Render(mode.String())
	truncate := lipgloss.NewStyle().MaxWidth(m.Width() -
		s.Selector.GetHorizontalFrameSize() -
		s.Selector.GetWidth())
	//nolint:errcheck
	fmt.Fprint(w,
		d.common.Zone.Mark(
			i.ID(),
			truncate.Render(fmt.Sprintf("%s%s%s",
				modeStr,
				size,
				name,
			)),
		),
	)
}

// Notes is the model for the Notes page showing user's file tree.
type Notes struct {
	common         common.Common
	selector       *selector.Selector
	code           *code.Code
	activeView     notesView
	userPath       string
	path           string
	currentItem    *NotesFileItem
	currentContent FileContentMsg
	lastSelected   []int
	lineNumber     bool
	cursor         int
}

// New creates a new Notes model.
func New(c common.Common) *Notes {
	n := &Notes{
		common:       c,
		code:         code.New(c, "", ""),
		activeView:   notesViewFiles,
		lastSelected: make([]int, 0),
		lineNumber:   true,
	}
	selector := selector.New(c, []selector.IdentifiableItem{}, NotesFileItemDelegate{&c})
	selector.SetShowFilter(false)
	selector.SetShowHelp(false)
	selector.SetShowPagination(false)
	selector.SetShowStatusBar(false)
	selector.SetShowTitle(false)
	selector.SetFilteringEnabled(false)
	selector.DisableQuitKeybindings()
	selector.KeyMap.NextPage = c.KeyMap.NextPage
	selector.KeyMap.PrevPage = c.KeyMap.PrevPage
	n.selector = selector
	n.code.ShowLineNumber = n.lineNumber
	return n
}

// Path implements common.TabComponent.
func (n *Notes) Path() string {
	path := n.path
	if path == "" {
		return ""
	}
	return path
}

// TabName returns the tab name.
func (n *Notes) TabName() string {
	return "Notes"
}

// SetSize implements common.Component.
func (n *Notes) SetSize(width, height int) {
	n.common.SetSize(width, height)
	n.selector.SetSize(width, height)
	n.code.SetSize(width, height)
}

// ShortHelp implements help.KeyMap.
func (n *Notes) ShortHelp() []key.Binding {
	k := n.selector.KeyMap
	switch n.activeView {
	case notesViewFiles:
		return []key.Binding{
			n.common.KeyMap.SelectItem,
			n.common.KeyMap.BackItem,
			k.CursorUp,
			k.CursorDown,
		}
	case notesViewContent:
		return []key.Binding{
			n.common.KeyMap.UpDown,
			n.common.KeyMap.BackItem,
		}
	default:
		return []key.Binding{}
	}
}

// FullHelp implements help.KeyMap.
func (n *Notes) FullHelp() [][]key.Binding {
	b := make([][]key.Binding, 0)
	copyKey := n.common.KeyMap.Copy
	switch n.activeView {
	case notesViewFiles:
		copyKey.SetHelp("c", "copy name")
		k := n.selector.KeyMap
		b = append(b, [][]key.Binding{
			{
				n.common.KeyMap.SelectItem,
				n.common.KeyMap.BackItem,
			},
			{
				k.CursorUp,
				k.CursorDown,
				k.NextPage,
				k.PrevPage,
			},
			{
				k.GoToStart,
				k.GoToEnd,
			},
		}...)
	case notesViewContent:
		if !n.code.UseGlamour {
			b = append(b, []key.Binding{lineNo})
		}
		copyKey.SetHelp("c", "copy content")
		k := n.code.KeyMap
		b = append(b, []key.Binding{
			n.common.KeyMap.BackItem,
		})
		b = append(b, [][]key.Binding{
			{
				k.PageDown,
				k.PageUp,
				k.HalfPageDown,
				k.HalfPageUp,
			},
			{
				k.Down,
				k.Up,
				n.common.KeyMap.GotoTop,
				n.common.KeyMap.GotoBottom,
			},
		}...)
	}
	return append(b, []key.Binding{copyKey})
}

// Init implements tea.Model.
func (n *Notes) Init() tea.Cmd {
	// Load user info
	ctx := n.common.Context()
	be := n.common.Backend()
	pk := n.common.PublicKey()

	if pk == nil {
		// Anonymous user - no notes directory
		n.common.Logger.Debug("notes: no public key, showing empty list")
		n.activeView = notesViewFiles
		return tea.Batch(n.selector.Init(), n.setItems([]selector.IdentifiableItem{}))
	}

	user, err := be.UserByPublicKey(ctx, pk)
	if err != nil {
		// User not found in users_path - show empty list
		n.common.Logger.Debugf("notes: user not found by public key: %v", err)
		n.activeView = notesViewFiles
		return tea.Batch(n.selector.Init(), n.setItems([]selector.IdentifiableItem{}))
	}

	// User path is in SOFT_SERVE_USER_HOME/{username}/
	cfg := n.common.Config()
	if cfg != nil {
		usersPath := getUsersPath(cfg.DataPath)
		if usersPath != "" {
			n.userPath = filepath.Join(usersPath, user.Username())
		}
	}

	// If userPath is still empty, show empty list
	if n.userPath == "" {
		n.common.Logger.Debug("notes: userPath is empty, showing empty list")
		n.activeView = notesViewFiles
		return tea.Batch(n.selector.Init(), n.setItems([]selector.IdentifiableItem{}))
	}

	n.common.Logger.Debugf("notes: userPath set to %s", n.userPath)

	n.path = ""
	n.currentItem = nil
	n.lastSelected = make([]int, 0)
	n.code.UseGlamour = false

	// Load files
	return tea.Batch(n.selector.Init(), n.loadFilesCmd())
}

// loadFilesCmd loads files and returns FileItemsMsg directly.
func (n *Notes) loadFilesCmd() tea.Cmd {
	return func() tea.Msg {
		return n.updateFilesMsg()
	}
}

// updateFilesMsg returns the file items message.
func (n *Notes) updateFilesMsg() FileItemsMsg {
	files := make([]selector.IdentifiableItem, 0)
	dirs := make([]selector.IdentifiableItem, 0)

	if n.userPath == "" {
		return FileItemsMsg{}
	}

	currentPath := filepath.Join(n.userPath, n.path)

	entries, err := os.ReadDir(currentPath)
	if err != nil {
		// Log the error for debugging
		n.common.Logger.Debugf("notes: failed to read directory %s: %v", currentPath, err)
		return FileItemsMsg{}
	}

	n.common.Logger.Debugf("notes: reading directory %s, found %d entries", currentPath, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		// Skip hidden files/directories (starting with .)
		if strings.HasPrefix(name, ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		item := NotesFileItem{
			name:  name,
			path:  filepath.Join(currentPath, name),
			isDir: entry.IsDir(),
			size:  info.Size(),
			mode:  info.Mode(),
		}

		if entry.IsDir() {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}

	// Sort: directories first, then files, alphabetically within each group
	return FileItemsMsg(append(dirs, files...))
}

// getUsersPath returns the path to users directory.
func getUsersPath(dataPath string) string {
	// Check for SOFT_SERVE_USER_HOME env
	if envPath := os.Getenv("SOFT_SERVE_USER_HOME"); envPath != "" {
		return expandPath(envPath)
	}
	// Default to {DataPath}/users
	return filepath.Join(dataPath, "users")
}

// expandPath expands ~ and environment variables in a path.
func expandPath(path string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		path = filepath.Join(home, path[2:])
	}
	return os.ExpandEnv(path)
}

// Update implements tea.Model.
func (n *Notes) Update(msg tea.Msg) (common.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0)
	switch msg := msg.(type) {
	case FileItemsMsg:
		cmds = append(cmds,
			n.selector.SetItems(msg),
		)
		n.activeView = notesViewFiles
		if n.cursor >= 0 {
			n.selector.Select(n.cursor)
			n.cursor = -1
		}
	case FileContentMsg:
		n.activeView = notesViewContent
		n.currentContent = msg
		n.code.UseGlamour = common.IsFileMarkdown(n.currentContent.content, n.currentContent.ext)
		cmds = append(cmds, n.code.SetContent(msg.content, msg.ext))
		n.code.GotoTop()
	case selector.SelectMsg:
		switch sel := msg.IdentifiableItem.(type) {
		case NotesFileItem:
			n.currentItem = &sel
			n.path = filepath.Join(n.path, sel.name)
			if sel.isDir {
				cmds = append(cmds, n.selectDirCmd)
			} else {
				cmds = append(cmds, n.selectFileCmd)
			}
		}
	case tea.KeyPressMsg:
		switch n.activeView {
		case notesViewFiles:
			switch {
			case key.Matches(msg, n.common.KeyMap.SelectItem):
				cmds = append(cmds, n.selector.SelectItemCmd)
			case key.Matches(msg, n.common.KeyMap.BackItem):
				cmds = append(cmds, n.deselectItemCmd())
			}
		case notesViewContent:
			switch {
			case key.Matches(msg, n.common.KeyMap.BackItem):
				cmds = append(cmds, n.deselectItemCmd())
			case key.Matches(msg, n.common.KeyMap.Copy):
				cmds = append(cmds, copyCmd(n.currentContent.content, "File contents copied to clipboard"))
			case key.Matches(msg, lineNo) && !n.code.UseGlamour:
				n.lineNumber = !n.lineNumber
				n.code.ShowLineNumber = n.lineNumber
				cmds = append(cmds, n.code.SetContent(n.currentContent.content, n.currentContent.ext))
			}
		}
	case tea.WindowSizeMsg:
		n.SetSize(msg.Width, msg.Height)
		switch n.activeView {
		case notesViewFiles:
			cmds = append(cmds, n.loadFilesCmd())
		case notesViewContent:
			if n.currentContent.content != "" {
				m, cmd := n.code.Update(msg)
				n.code = m.(*code.Code)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
	}
	switch n.activeView {
	case notesViewFiles:
		m, cmd := n.selector.Update(msg)
		n.selector = m.(*selector.Selector)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case notesViewContent:
		m, cmd := n.code.Update(msg)
		n.code = m.(*code.Code)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return n, tea.Batch(cmds...)
}

// View implements tea.Model.
func (n *Notes) View() string {
	switch n.activeView {
	case notesViewFiles:
		return n.selector.View()
	case notesViewContent:
		return n.code.View()
	default:
		return ""
	}
}

// SpinnerID implements common.TabComponent.
func (n *Notes) SpinnerID() int {
	return 0 // No spinner used
}

// StatusBarValue returns the status bar value.
func (n *Notes) StatusBarValue() string {
	p := n.path
	if p == "" {
		return " "
	}
	return p
}

// StatusBarInfo returns the status bar info.
func (n *Notes) StatusBarInfo() string {
	switch n.activeView {
	case notesViewFiles:
		return fmt.Sprintf("# %d/%d", n.selector.Index()+1, len(n.selector.VisibleItems()))
	case notesViewContent:
		return common.ScrollPercent(n.code.ScrollPosition())
	default:
		return ""
	}
}

func (n *Notes) selectDirCmd() tea.Msg {
	if n.currentItem != nil && n.currentItem.isDir {
		n.lastSelected = append(n.lastSelected, n.selector.Index())
		n.cursor = 0
		return n.updateFilesMsg()
	}
	return common.ErrorMsg(errNoFileSelected)
}

func (n *Notes) selectFileCmd() tea.Msg {
	i := n.currentItem
	if i != nil && !i.isDir {
		// Read file content
		data, err := os.ReadFile(i.path)
		if err != nil {
			n.path = filepath.Dir(n.path)
			return common.ErrorMsg(err)
		}

		// Check if binary
		if isBinary(data) {
			n.path = filepath.Dir(n.path)
			return common.ErrorMsg(errBinaryFile)
		}

		n.lastSelected = append(n.lastSelected, n.selector.Index())
		ext := filepath.Ext(i.name)
		return FileContentMsg{string(data), ext}
	}
	return common.ErrorMsg(errNoFileSelected)
}

func (n *Notes) deselectItemCmd() tea.Cmd {
	n.path = filepath.Dir(n.path)
	if n.path == "." {
		n.path = ""
	}
	index := 0
	if len(n.lastSelected) > 0 {
		index = n.lastSelected[len(n.lastSelected)-1]
		n.lastSelected = n.lastSelected[:len(n.lastSelected)-1]
	}
	n.cursor = index
	n.activeView = notesViewFiles
	n.code.UseGlamour = false
	return n.loadFilesCmd()
}

func (n *Notes) setItems(items []selector.IdentifiableItem) tea.Cmd {
	return func() tea.Msg {
		return FileItemsMsg(items)
	}
}

func copyCmd(content string, msg string) tea.Cmd {
	return tea.Sequence(
		tea.SetClipboard(content),
		tea.Println(msg),
	)
}

// isBinary checks if data appears to be binary content.
func isBinary(data []byte) bool {
	// Check first 512 bytes for null bytes (common binary indicator)
	maxCheck := 512
	if len(data) < maxCheck {
		maxCheck = len(data)
	}
	for i := 0; i < maxCheck; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}
