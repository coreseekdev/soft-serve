package notes

import (
	"fmt"
	"io"
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
)

const (
	noFilesContent = "No visible files in your directory."
)

// Notes is the model for the Notes page showing user's file tree.
type Notes struct {
	common       common.Common
	username     string
	userPath     string
	currentPath  string   // Current directory path in the file tree
	pathStack    []string // Stack for navigation history
	files        []fileItem
	fileList     *selector.Selector
	fileContent  *code.Code
	viewingFile  bool   // Whether we're viewing a file content
	currentFile  string // Current file name being viewed
}

type fileItem struct {
	name  string
	path  string
	isDir bool
}

// FilterValue implements list.Item.
func (f fileItem) FilterValue() string {
	return f.name
}

// Title implements list.DefaultItem.
func (f fileItem) Title() string {
	return f.name
}

// Description implements list.DefaultItem.
func (f fileItem) Description() string {
	if f.isDir {
		return "directory"
	}
	return "file"
}

// ID implements selector.IdentifiableItem.
func (f fileItem) ID() string {
	return f.name
}

// fileItemDelegate is the delegate for file items.
type fileItemDelegate struct {
	common.Common
}

// NewFileItemDelegate creates a new file item delegate.
func NewFileItemDelegate(c common.Common) fileItemDelegate {
	return fileItemDelegate{Common: c}
}

// Height implements list.ItemDelegate.
func (d fileItemDelegate) Height() int {
	return 1
}

// Spacing implements list.ItemDelegate.
func (d fileItemDelegate) Spacing() int {
	return 0
}

// Update implements list.ItemDelegate.
func (d fileItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

// Render implements list.ItemDelegate.
func (d fileItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	fi, ok := item.(fileItem)
	if !ok {
		return
	}

	s := d.Styles.Tree
	var name string
	if fi.isDir {
		if index == m.Index() {
			name = s.Active.FileDir.Render(fi.name)
			fmt.Fprint(w, s.Selector.Render(">"))
		} else {
			name = s.Normal.FileDir.Render(fi.name)
			fmt.Fprint(w, s.Selector.Render(" "))
		}
	} else {
		if index == m.Index() {
			name = s.Active.FileName.Render(fi.name)
			fmt.Fprint(w, s.Selector.Render(">"))
		} else {
			name = s.Normal.FileName.Render(fi.name)
			fmt.Fprint(w, s.Selector.Render(" "))
		}
	}

	fmt.Fprint(w, name)
}

// New creates a new Notes model.
func New(c common.Common) *Notes {
	n := &Notes{
		common:    c,
		pathStack: make([]string, 0),
	}

	// Initialize file list
	selector := selector.New(c, []selector.IdentifiableItem{}, NewFileItemDelegate(c))
	selector.SetShowTitle(false)
	selector.SetShowHelp(false)
	selector.SetShowStatusBar(false)
	selector.DisableQuitKeybindings()
	n.fileList = selector

	// Initialize code for file content display
	fileContent := code.New(c, "", "")
	fileContent.UseGlamour = false
	n.fileContent = fileContent

	return n
}

// SetSize implements common.Component.
func (n *Notes) SetSize(width, height int) {
	n.common.SetSize(width, height)
	n.fileList.SetSize(width, height-2)
	n.fileContent.SetSize(width, height-2)
}

// ShortHelp implements help.KeyMap.
func (n *Notes) ShortHelp() []key.Binding {
	if n.viewingFile {
		ck := n.fileContent.KeyMap
		return []key.Binding{
			n.common.KeyMap.BackItem,
			ck.Up,
			ck.Down,
		}
	}
	k := n.fileList.KeyMap
	return []key.Binding{
		n.common.KeyMap.SelectItem,
		n.common.KeyMap.BackItem,
		k.CursorUp,
		k.CursorDown,
	}
}

// FullHelp implements help.KeyMap.
func (n *Notes) FullHelp() [][]key.Binding {
	if n.viewingFile {
		k := n.fileContent.KeyMap
		return [][]key.Binding{
			{n.common.KeyMap.BackItem},
			{k.Up, k.Down},
			{k.PageDown, k.PageUp},
			{k.HalfPageDown, k.HalfPageUp},
		}
	}
	k := n.fileList.KeyMap
	return [][]key.Binding{
		{n.common.KeyMap.SelectItem, n.common.KeyMap.BackItem},
		{k.CursorUp, k.CursorDown},
		{k.NextPage, k.PrevPage, k.GoToStart, k.GoToEnd},
	}
}

// Init implements tea.Model.
func (n *Notes) Init() tea.Cmd {
	// Load user info
	ctx := n.common.Context()
	be := n.common.Backend()
	pk := n.common.PublicKey()

	if pk == nil {
		// Anonymous user - no notes directory
		return n.fileList.Init()
	}

	user, err := be.UserByPublicKey(ctx, pk)
	if err != nil {
		return n.fileList.Init()
	}

	n.username = user.Username()

	// Load user directory files (for filestore mode)
	// User path is in SOFT_SERVE_USER_HOME/{username}/
	cfg := n.common.Config()
	if cfg != nil {
		usersPath := getUsersPath(cfg.DataPath)
		if usersPath != "" {
			n.userPath = filepath.Join(usersPath, n.username)
			n.currentPath = n.userPath
			n.loadFiles()
		}
	}

	return tea.Batch(n.fileList.Init())
}

// getUsersPath returns the path to users directory.
func getUsersPath(dataPath string) string {
	// Check for SOFT_SERVE_USER_HOME env
	if envPath := os.Getenv("SOFT_SERVE_USER_HOME"); envPath != "" {
		return envPath
	}
	// Default to {DataPath}/users
	return filepath.Join(dataPath, "users")
}

// loadFiles loads the files in the current directory.
func (n *Notes) loadFiles() {
	if n.currentPath == "" {
		n.files = []fileItem{}
		return
	}

	entries, err := os.ReadDir(n.currentPath)
	if err != nil {
		n.files = []fileItem{}
		return
	}

	// Separate dirs and files, sort each
	var dirs, files []fileItem
	for _, entry := range entries {
		name := entry.Name()
		// Skip hidden files/directories (starting with .)
		if strings.HasPrefix(name, ".") {
			continue
		}

		item := fileItem{
			name:  name,
			path:  filepath.Join(n.currentPath, name),
			isDir: entry.IsDir(),
		}
		if entry.IsDir() {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}

	// Combine: dirs first, then files
	n.files = make([]fileItem, 0, len(dirs)+len(files))
	n.files = append(n.files, dirs...)
	n.files = append(n.files, files...)

	// Convert to selector items
	items := make([]selector.IdentifiableItem, len(n.files))
	for i, f := range n.files {
		items[i] = f
	}
	n.fileList.SetItems(items)
}

// Update implements tea.Model.
func (n *Notes) Update(msg tea.Msg) (common.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		n.SetSize(msg.Width, msg.Height)
	case tea.KeyPressMsg:
		if n.viewingFile {
			switch {
			case key.Matches(msg, n.common.KeyMap.BackItem):
				n.viewingFile = false
				n.currentFile = ""
			}
		} else {
			switch {
			case key.Matches(msg, n.common.KeyMap.SelectItem):
				cmds = append(cmds, n.selectItemCmd)
			case key.Matches(msg, n.common.KeyMap.BackItem):
				cmds = append(cmds, n.goBackCmd)
			}
		}
	case selector.SelectMsg:
		if !n.viewingFile {
			cmds = append(cmds, n.selectItemCmd)
		}
	}

	// Update active component
	if n.viewingFile {
		c, cmd := n.fileContent.Update(msg)
		n.fileContent = c.(*code.Code)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else {
		s, cmd := n.fileList.Update(msg)
		n.fileList = s.(*selector.Selector)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return n, tea.Batch(cmds...)
}

func (n *Notes) selectItemCmd() tea.Msg {
	idx := n.fileList.Index()
	if idx < 0 || idx >= len(n.files) {
		return nil
	}

	item := n.files[idx]
	if item.isDir {
		// Navigate into directory
		n.pathStack = append(n.pathStack, n.currentPath)
		n.currentPath = item.path
		n.loadFiles()
		n.fileList.Select(0)
	} else {
		// View file content
		data, err := os.ReadFile(item.path)
		if err != nil {
			return common.ErrorMsg(err)
		}
		n.viewingFile = true
		n.currentFile = item.name
		// Detect file extension for syntax highlighting
		ext := filepath.Ext(item.name)
		n.fileContent.SetContent(string(data), ext)
	}
	return nil
}

func (n *Notes) goBackCmd() tea.Msg {
	if n.viewingFile {
		n.viewingFile = false
		n.currentFile = ""
		return nil
	}

	if len(n.pathStack) > 0 {
		// Go back to parent directory
		n.currentPath = n.pathStack[len(n.pathStack)-1]
		n.pathStack = n.pathStack[:len(n.pathStack)-1]
		n.loadFiles()
		n.fileList.Select(0)
	}
	return nil
}

// View implements tea.Model.
func (n *Notes) View() string {
	var content string

	// Build path breadcrumb
	var breadcrumb string
	if n.userPath != "" && strings.HasPrefix(n.currentPath, n.userPath) {
		rel, err := filepath.Rel(n.userPath, n.currentPath)
		if err == nil && rel != "." {
			breadcrumb = fmt.Sprintf(" ~/%s", rel)
		} else if rel == "." {
			breadcrumb = " ~"
		}
	}

	if n.viewingFile {
		// Viewing file content
		header := n.common.Styles.Repo.HeaderName.Render(n.currentFile)
		headerStyle := n.common.Styles.Repo.Header.Render(header)
		content = lipgloss.JoinVertical(lipgloss.Left,
			headerStyle,
			n.fileContent.View(),
		)
	} else if len(n.files) == 0 {
		content = n.common.Styles.NoContent.Render(noFilesContent)
	} else {
		content = n.fileList.View()
	}

	// Add path breadcrumb at the top
	pathLine := fmt.Sprintf("Notes: %s", breadcrumb)
	header := n.common.Styles.Tabs.Render(pathLine)

	view := lipgloss.JoinVertical(lipgloss.Left,
		header,
		content,
	)

	return view
}
