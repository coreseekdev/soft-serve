package me

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
	defaultNoContent = "No user information available."
)

type pane int

const (
	infoPane pane = iota
	filesPane
	lastPane
)

func (p pane) String() string {
	return []string{
		"Info",
		"Files",
	}[p]
}

// Me is the model for the Me page showing user info and files.
type Me struct {
	common     common.Common
	activePane pane
	username   string
	isAdmin    bool
	publicKeys []string
	userPath   string
	files      []fileItem
	fileList   *selector.Selector
	infoCode   *code.Code
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

// New creates a new Me model.
func New(c common.Common) *Me {
	m := &Me{
		common:     c,
		activePane: infoPane,
	}

	// Initialize file list
	selector := selector.New(c, []selector.IdentifiableItem{}, NewFileItemDelegate(c))
	selector.SetShowTitle(false)
	selector.SetShowHelp(false)
	selector.SetShowStatusBar(false)
	selector.DisableQuitKeybindings()
	m.fileList = selector

	// Initialize code for info display
	infoCode := code.New(c, "", "")
	infoCode.UseGlamour = true
	infoCode.NoContentStyle = c.Styles.NoContent.SetString(defaultNoContent)
	m.infoCode = infoCode

	return m
}

// SetSize implements common.Component.
func (m *Me) SetSize(width, height int) {
	m.common.SetSize(width, height)
	m.fileList.SetSize(width, height-2)
	m.infoCode.SetSize(width, height-2)
}

// ShortHelp implements help.KeyMap.
func (m *Me) ShortHelp() []key.Binding {
	kb := make([]key.Binding, 0)
	kb = append(kb, m.common.KeyMap.Section)
	if m.activePane == filesPane {
		kb = append(kb, m.fileList.KeyMap.CursorUp, m.fileList.KeyMap.CursorDown)
	} else {
		k := m.infoCode.KeyMap
		kb = append(kb, k.Up, k.Down)
	}
	return kb
}

// FullHelp implements help.KeyMap.
func (m *Me) FullHelp() [][]key.Binding {
	b := [][]key.Binding{
		{m.common.KeyMap.Section},
	}
	if m.activePane == filesPane {
		k := m.fileList.KeyMap
		b = append(b, []key.Binding{k.CursorUp, k.CursorDown})
		b = append(b, []key.Binding{k.NextPage, k.PrevPage})
	} else {
		k := m.infoCode.KeyMap
		b = append(b, []key.Binding{k.PageDown, k.PageUp})
		b = append(b, []key.Binding{k.HalfPageDown, k.HalfPageUp})
		b = append(b, []key.Binding{k.Down, k.Up})
	}
	return b
}

// Init implements tea.Model.
func (m *Me) Init() tea.Cmd {
	// Load user info
	ctx := m.common.Context()
	be := m.common.Backend()
	pk := m.common.PublicKey()

	if pk == nil {
		m.infoCode.SetContent("No authentication key found.\n\nYou are browsing as anonymous user.", "")
		return m.fileList.Init()
	}

	user, err := be.UserByPublicKey(ctx, pk)
	if err != nil {
		m.infoCode.SetContent(fmt.Sprintf("User not found: %v", err), "")
		return m.fileList.Init()
	}

	m.username = user.Username()
	m.isAdmin = user.IsAdmin()

	// Get public keys
	keys := user.PublicKeys()
	var keyStrs []string
	for _, k := range keys {
		keyStrs = append(keyStrs, fmt.Sprintf("  - Key type: %s", k.Type()))
	}
	m.publicKeys = keyStrs

	// Build info content as markdown
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# User: %s\n\n", m.username))
	sb.WriteString(fmt.Sprintf("**Admin:** %v\n\n", m.isAdmin))
	sb.WriteString("## Public Keys\n\n")
	for i, k := range m.publicKeys {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, k))
	}
	sb.WriteString("\n---\n\n")
	sb.WriteString(fmt.Sprintf("*Press Tab to switch to Files view*\n"))

	m.infoCode.SetContent(sb.String(), "md")

	// Load user directory files (for filestore mode)
	// User path is in SOFT_SERVE_USER_HOME/{username}/
	cfg := m.common.Config()
	if cfg != nil {
		usersPath := getVisibleUsersPath(cfg.DataPath)
		if usersPath != "" {
			m.userPath = filepath.Join(usersPath, m.username)
			m.loadFiles()
		}
	}

	return tea.Batch(m.fileList.Init())
}

// getVisibleUsersPath returns the path to users directory.
func getVisibleUsersPath(dataPath string) string {
	// Check for SOFT_SERVE_USER_HOME env
	if envPath := os.Getenv("SOFT_SERVE_USER_HOME"); envPath != "" {
		return envPath
	}
	// Default to {DataPath}/users
	return filepath.Join(dataPath, "users")
}

// loadFiles loads the files in the user's directory.
func (m *Me) loadFiles() {
	if m.userPath == "" {
		return
	}

	entries, err := os.ReadDir(m.userPath)
	if err != nil {
		m.files = []fileItem{}
		return
	}

	m.files = make([]fileItem, 0)
	for _, entry := range entries {
		name := entry.Name()
		// Skip hidden files/directories (starting with .)
		if strings.HasPrefix(name, ".") {
			continue
		}

		m.files = append(m.files, fileItem{
			name:  name,
			path:  filepath.Join(m.userPath, name),
			isDir: entry.IsDir(),
		})
	}

	// Convert to selector items
	items := make([]selector.IdentifiableItem, len(m.files))
	for i, f := range m.files {
		items[i] = f
	}
	m.fileList.SetItems(items)
}

// Update implements tea.Model.
func (m *Me) Update(msg tea.Msg) (common.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.common.KeyMap.Section):
			// Switch between info and files panes
			m.activePane = (m.activePane + 1) % lastPane
		}
	}

	// Update active pane
	switch m.activePane {
	case filesPane:
		s, cmd := m.fileList.Update(msg)
		m.fileList = s.(*selector.Selector)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case infoPane:
		c, cmd := m.infoCode.Update(msg)
		m.infoCode = c.(*code.Code)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m *Me) View() string {
	var content string

	// Tab indicator
	tabStyle := m.common.Styles.TabInactive
	if m.activePane == infoPane {
		tabStyle = m.common.Styles.TabActive
	}
	tabIndicator := fmt.Sprintf(" %s | Files ", tabStyle.Render("Info"))

	switch m.activePane {
	case infoPane:
		content = m.infoCode.View()
	case filesPane:
		if len(m.files) == 0 {
			content = m.common.Styles.NoContent.Render("No visible files in your directory.")
		} else {
			content = m.fileList.View()
		}
	}

	// Add tab indicator at the top
	view := lipgloss.JoinVertical(lipgloss.Left,
		m.common.Styles.Tabs.Render(tabIndicator),
		content,
	)

	return view
}
