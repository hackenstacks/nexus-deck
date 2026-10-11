package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Library Styles
var (
	libraryTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#DAA520")). // Goldenrod
			Padding(1, 4).
			Align(lipgloss.Center).
			Bold(true)

	libraryItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(lipgloss.Color("#E6E6FA")) // Lavender

	librarySelectedItemStyle = lipgloss.NewStyle().
					PaddingLeft(2).
					Foreground(lipgloss.Color("#FFD700")). // Gold
					Border(lipgloss.RoundedBorder(), false, false, false, true).
					BorderForeground(lipgloss.Color("#FFD700"))

	libraryHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#757575")). // Grey
				PaddingTop(1)

	libraryBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#DAA520")). // Goldenrod
				Padding(1, 2).
				Width(80)

	libraryErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF0000")). // Red
				Bold(true)

	libraryFileDetailStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFFFFF")). // White
					Padding(1, 2).
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("#DAA520"))
)

type libraryState int

const (
	libraryListView libraryState = iota
	libraryDetailView
	librarySharePasswordView // New state for entering sharing password
)

type libraryModel struct {
	user        *User
	state       libraryState
	fileList    list.Model
	selectedFile *LibraryFile
	passwordInput textinput.Model // For sharing password
	outputPathInput textinput.Model // For sharing output path
	err         error
	deleteSuccess bool
	shareSuccess bool
}

func initialLibraryModel(u *User) libraryModel {
	// Initialize file list
	files, err := ListLibraryFiles(u)
	if err != nil {
		fmt.Printf("Error loading library files: %v\n", err)
	}
	items := make([]list.Item, len(files))
	for i, file := range files {
		items[i] = libraryListItem{file: file, title: fmt.Sprintf("%d. %s", i+1, file.Name)}
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = librarySelectedItemStyle
	delegate.Styles.SelectedDesc = librarySelectedItemStyle.Copy().Faint(true)
	delegate.Styles.NormalTitle = libraryItemStyle
	delegate.Styles.NormalDesc = libraryItemStyle.Copy().Faint(true)

	l := list.New(items, delegate, 76, 20)
	l.Title = "Your Library 📚"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = libraryTitleStyle

	passwordInput := textinput.New()
	passwordInput.Placeholder = "Enter sharing password"
	passwordInput.EchoMode = textinput.EchoPassword
	passwordInput.EchoCharacter = '•'
	passwordInput.CharLimit = 100
	passwordInput.Width = 60

	outPathInput := textinput.New()
	outPathInput.Placeholder = "Enter output path for shared JSON (e.g., /home/user/shared_file.json)"
	outPathInput.CharLimit = 200
	outPathInput.Width = 80


	return libraryModel{
		user:     u,
		state:    libraryListView,
		fileList: l,
		passwordInput: passwordInput,
		outputPathInput: outPathInput,
	}
}

// libraryItem represents a single file in the list.
type libraryListItem struct {
	file LibraryFile
	title string
}

func (i libraryListItem) FilterValue() string { return i.file.Name }
func (i libraryListItem) Title() string       { return i.title }
func (i libraryListItem) Description() string  { return fmt.Sprintf("Type: %s, ID: %s", i.file.Type, i.file.ID) }

func (m libraryModel) Init() tea.Cmd { return nil }

func (m libraryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := libraryBoxStyle.GetHorizontalFrameSize(), libraryBoxStyle.GetVerticalFrameSize()
		m.fileList.SetSize(msg.Width-h, msg.Height-v)
		m.passwordInput.Width = msg.Width - h - 10
		m.outputPathInput.Width = msg.Width - h - 10

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit
		case "ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5", "ctrl+6", "ctrl+7", "ctrl+8", "ctrl+9":
			if m.state == libraryListView {
				idx := int(msg.String()[5] - '1')
				if idx >= 0 && idx < len(m.fileList.Items()) {
					m.fileList.Select(idx)
					// Auto-select and enter detail view
					selectedItem := m.fileList.Items()[idx].(libraryListItem)
					m.selectedFile = &selectedItem.file
					m.state = libraryDetailView
					return m, nil
				}
			}
		case "esc":
			m.err = nil // Clear error on escape
			m.deleteSuccess = false // Clear success message
			m.shareSuccess = false // Clear share success message
			if m.state == libraryListView {
				return m, func() tea.Msg { return BackToMainAppMsg{} }
			}
			m.state = libraryListView
			m.fileList.SetItems(m.loadLibraryItems()) // Reload list
			m.selectedFile = nil
			m.passwordInput.Reset()
			m.outputPathInput.Reset()
			m.passwordInput.Blur()
			m.outputPathInput.Blur()
			return m, nil

		case "enter":
			switch m.state {
			case libraryListView:
				selectedItem := m.fileList.SelectedItem()
				if selectedItem == nil {
					return m, nil // No item selected
				}
				item := selectedItem.(libraryListItem)
				m.selectedFile = &item.file
				m.state = libraryDetailView
				return m, nil
			case librarySharePasswordView:
				if m.passwordInput.Focused() {
					if m.passwordInput.Value() == "" {
						m.err = fmt.Errorf("sharing password cannot be empty")
						return m, nil
					}
					m.passwordInput.Blur()
					m.outputPathInput.Focus()
					return m, nil
				} else if m.outputPathInput.Focused() {
					if m.outputPathInput.Value() == "" {
						m.err = fmt.Errorf("output path cannot be empty")
						return m, nil
					}
					                    // Load file content
										_, fileContent, err := LoadFileFromLibrary(m.user, m.selectedFile.ID)
										if err != nil {
											m.err = fmt.Errorf("failed to load file content for sharing: %w", err)
											return m, nil
										}
					
										// Encrypt and create shared JSON
										outputPath := m.outputPathInput.Value()
										sharingPassword := m.passwordInput.Value()
					
										if err := EncryptAndMarshalSharedFile(outputPath, m.selectedFile, fileContent, sharingPassword); err != nil {
											m.err = fmt.Errorf("failed to create shared file: %w", err)
										} else {
											m.shareSuccess = true
											m.state = libraryListView
											m.passwordInput.Reset()
											m.outputPathInput.Reset()
											m.passwordInput.Blur()
											m.outputPathInput.Blur()
										}
										return m, nil
				}
			}
		case "ctrl+d": // Delete file
			if m.state == libraryListView {
				selectedItem := m.fileList.SelectedItem()
				if selectedItem == nil {
					m.err = fmt.Errorf("no file selected to delete")
					return m, nil
				}
				fileToDelete := selectedItem.(libraryListItem).file
				if err := DeleteFileFromLibrary(m.user, fileToDelete.ID); err != nil {
					m.err = fmt.Errorf("failed to delete file: %w", err)
				} else {
					m.deleteSuccess = true
					m.fileList.SetItems(m.loadLibraryItems()) // Reload list
				}
				return m, nil
			}
		case "ctrl+v": // View file content (simple text for now)
			if m.state == libraryDetailView && m.selectedFile != nil {
				// Load and display content
				_, content, err := LoadFileFromLibrary(m.user, m.selectedFile.ID)
				if err != nil {
					m.err = fmt.Errorf("failed to load file content: %w", err)
					return m, nil
				}
				// For now, display text content directly. Binary files will show garbage.
				m.selectedFile.OriginalPath = string(content) // Abusing OriginalPath to show content temporarily
				return m, nil
			}
		case "ctrl+s": // Share file
			if m.state == libraryDetailView && m.selectedFile != nil {
				m.err = nil
				m.shareSuccess = false
				m.state = librarySharePasswordView
				m.passwordInput.Focus()
				return m, nil
			}
		}
	}

	// Delegate updates to appropriate sub-components
	switch m.state {
	case libraryListView:
		m.fileList, cmd = m.fileList.Update(msg)
		cmds = append(cmds, cmd)
	case librarySharePasswordView:
		m.passwordInput, cmd = m.passwordInput.Update(msg)
		cmds = append(cmds, cmd)
		m.outputPathInput, cmd = m.outputPathInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m libraryModel) View() string {
	var s string
	var help string
	var status string

	titleStyle := TitleStyle()
	promptStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Bold(true)

	if m.err != nil {
		status = ErrorStyle().Render(fmt.Sprintf("⛔ Error: %v", m.err)) + "\n"
	} else if m.deleteSuccess {
		status = SuccessStyle().Render(" ✅ File deleted successfully!") + "\n"
	} else if m.shareSuccess {
		status = SuccessStyle().Render(" ✅ File shared successfully!") + "\n"
	}

	switch m.state {
	case libraryListView:
		s = m.fileList.View()
		help = "\n ↑/↓ move • enter view details • ctrl+d delete • esc back • ctrl+q quit"
	case libraryDetailView:
		if m.selectedFile == nil {
			s = "No file selected."
		} else {
			s = titleStyle.Render(" 📋 FILE DETAILS ") + "\n\n" +
				fmt.Sprintf("Name: %s\n", m.selectedFile.Name) +
				fmt.Sprintf("Type: %s\n", m.selectedFile.Type) +
				fmt.Sprintf("ID: %s\n", m.selectedFile.ID) +
				fmt.Sprintf("Timestamp: %s\n", m.selectedFile.Timestamp.Format("2006-01-02 15:04:05")) +
				fmt.Sprintf("Stored Path: %s\n", m.selectedFile.StoredPath)
			if m.selectedFile.Type == "text" || m.selectedFile.Type == "chat_log" || m.selectedFile.Type == "json" {
				s += "\n-- Content Preview --\n" + m.selectedFile.OriginalPath
				help = "ctrl+v view content • ctrl+s share • esc back • ctrl+q quit"
			} else {
				help = "ctrl+s share • esc back • ctrl+q quit"
			}
		}
	case librarySharePasswordView:
		s = titleStyle.Render(" 🔗 SHARE FILE ") + "\n\n" +
			fmt.Sprintf("Sharing: %s (ID: %s)\n\n", m.selectedFile.Name, m.selectedFile.ID) +
			promptStyle.Render("Sharing Password:") + "\n" +
			m.passwordInput.View() + "\n\n" +
			promptStyle.Render("Output Path:") + "\n" +
			m.outputPathInput.View() + "\n\n"
		help = "tab cycle fields • enter to confirm/save • esc back • q quit"
	}

	return BorderedBox(true).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			s,
			status,
			MutedStyle().Render(help),
		),
	)
}

// Helper to load library items for the list.
func (m *libraryModel) loadLibraryItems() []list.Item {
	files, err := ListLibraryFiles(m.user)
	if err != nil {
		m.err = fmt.Errorf("error loading library files for list: %w", err)
		return []list.Item{}
	}
	items := make([]list.Item, len(files))
	for i, file := range files {
		items[i] = libraryListItem{file: file}
	}
	return items
}