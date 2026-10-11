package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	vaultTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ADD8E6")).
			Bold(true).
			Padding(1, 4).
			Align(lipgloss.Center).
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#ADD8E6"))

	vaultSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#87FF87")).Bold(true)
	vaultErrorStyle   = chatErrorStyle
	vaultHelpStyle    = chatHelpStyle
	vaultItemStyle    = chatItemStyle
	vaultCursorStyle  = chatSelectedItemStyle
)

type vaultState int

const (
	vaultBrowse vaultState = iota
	vaultIncoming
	vaultExport // password prompt for outbound share
	vaultImport // preview + password for inbound
	vaultUpload // prompt for file path to upload
)

type vaultModel struct {
	user         *User
	items        []VaultItem
	cursor       int
	state        vaultState
	input        textinput.Model    // password / filename entry
	previewPkg   *NexusVaultPackage // package being reviewed before import
	importPath   string             // path to .nexusvault file being imported
	exportItemID string             // ID of item being exported
	err          error
	status       string
	width        int
	height       int
}

func initialVaultModel(u *User) vaultModel {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.CharLimit = 255
	ti.Width = 60
	// Do NOT focus on init — only focus when entering an input state

	items, _ := LoadVaultItems(u)

	return vaultModel{
		user:  u,
		items: items,
		state: vaultBrowse,
		input: ti,
	}
}

func (m vaultModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m vaultModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			if m.state == vaultBrowse {
				return m, tea.Quit
			}
		case "?":
			if m.state == vaultBrowse {
				m.status = "[ctrl+u]pload file  [ctrl+e]xport  [ctrl+i]mport  [ctrl+d]elete  [↑/↓] navigate  [ESC] back"
				return m, nil
			}
		case "esc":
			if m.state == vaultBrowse {
				m.err = nil
				m.status = ""
				return m, nil
			}
			m.state = vaultBrowse
			m.input.Reset()
			m.err = nil
			m.status = ""
			m.previewPkg = nil
			return m, nil

		case "up", "ctrl+k":
			if m.state == vaultBrowse {
				if m.cursor > 0 {
					m.cursor--
				}
			}
		case "down", "ctrl+j":
			if m.state == vaultBrowse {
				if m.cursor < len(m.items)-1 {
					m.cursor++
				}
			}

		case "ctrl+u":
			if m.state == vaultBrowse {
				m.state = vaultUpload
				m.input.Placeholder = "Enter path to file to upload"
				m.input.EchoMode = textinput.EchoNormal
				m.input.Focus()
				return m, nil
			}
		case "ctrl+e":
			if m.state == vaultBrowse && len(m.items) > 0 {
				selected := m.items[m.cursor]
				if selected.Type == "folder" {
					m.err = fmt.Errorf("folders cannot be exported yet")
					return m, nil
				}
				m.exportItemID = selected.ID
				m.state = vaultExport
				m.input.Placeholder = "Enter share password"
				m.input.EchoMode = textinput.EchoPassword
				m.input.Focus()
				return m, nil
			}
		case "ctrl+d":
			if m.state == vaultBrowse && len(m.items) > 0 {
				selected := m.items[m.cursor]
				err := DeleteVaultItem(m.user, selected.ID)
				if err != nil {
					m.err = err
				} else {
					m.status = fmt.Sprintf("✅ Deleted: %s", selected.Name)
					m.items, _ = LoadVaultItems(m.user)
					if m.cursor >= len(m.items) && m.cursor > 0 {
						m.cursor = len(m.items) - 1
					}
				}
				return m, nil
			}
		case "ctrl+i":
			if m.state == vaultBrowse {
				m.state = vaultImport
				m.input.Placeholder = "Enter path to .nexusvault file"
				m.input.EchoMode = textinput.EchoNormal
				m.input.Focus()
				m.previewPkg = nil
				return m, nil
			}

		case "enter":
			switch m.state {
			case vaultUpload:
				path := m.input.Value()
				if path == "" {
					return m, nil
				}
				info, err := os.Stat(path)
				if err != nil {
					m.err = err
					return m, nil
				}
				content, err := os.ReadFile(path)
				if err != nil {
					m.err = err
					return m, nil
				}

				item := VaultItem{
					ID:        GenerateRandomID(),
					ParentID:  VaultRootID,
					Name:      filepath.Base(path),
					Type:      "file",
					MimeType:  "application/octet-stream", // simple default
					Size:      info.Size(),
					CreatedAt: time.Now().Unix(),
				}

				err = SaveVaultFile(m.user, item, content)
				if err != nil {
					m.err = err
					return m, nil
				}

				// Update index
				m.items = append(m.items, item)
				err = SaveVaultIndex(m.user, m.items)
				if err != nil {
					m.err = err
					return m, nil
				}

				m.status = fmt.Sprintf("✅ Uploaded: %s", item.Name)
				m.state = vaultBrowse
				m.input.Reset()
				return m, nil

			case vaultExport:
				pw := m.input.Value()
				if pw == "" {
					return m, nil
				}
				pkgData, err := ExportVaultPackage(m.user, m.exportItemID, pw)
				if err != nil {
					m.err = err
					return m, nil
				}

				// Find item name
				var name string
				for _, it := range m.items {
					if it.ID == m.exportItemID {
						name = it.Name
						break
					}
				}
				filename := name + ".nexusvault"
				err = os.WriteFile(filename, pkgData, 0644)
				if err != nil {
					m.err = err
					return m, nil
				}

				m.status = fmt.Sprintf("✅ Exported to %s", filename)
				m.state = vaultBrowse
				m.input.Reset()
				return m, nil

			case vaultImport:
				if m.previewPkg == nil {
					// Step 1: Load and show preview
					path := m.input.Value()
					data, err := os.ReadFile(path)
					if err != nil {
						m.err = err
						return m, nil
					}
					var pkg NexusVaultPackage
					if err := json.Unmarshal(data, &pkg); err != nil {
						m.err = fmt.Errorf("invalid .nexusvault file")
						return m, nil
					}
					m.previewPkg = &pkg
					m.importPath = path
					m.input.Reset()
					m.input.Placeholder = "Enter share password to decrypt"
					m.input.EchoMode = textinput.EchoPassword
					return m, nil
				} else {
					// Step 2: Decrypt and import
					pw := m.input.Value()
					data, _ := os.ReadFile(m.importPath)
					item, err := ImportVaultPackage(m.user, data, pw)
					if err != nil {
						m.err = err
						return m, nil
					}
					m.status = fmt.Sprintf("✅ Imported: %s", item.Name)
					m.items, _ = LoadVaultItems(m.user)
					m.state = vaultBrowse
					m.input.Reset()
					m.previewPkg = nil
					return m, nil
				}
			}
		}
	}

	// Only update input when in a state that needs text entry
	if m.state != vaultBrowse {
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m vaultModel) View() string {
	var s strings.Builder

	titleStyle := TitleStyle()
	s.WriteString(titleStyle.Render(" 🏛️  THE VAULT — SOVEREIGN STORAGE ") + "\n\n")

	if m.err != nil {
		s.WriteString(ErrorStyle().Render(fmt.Sprintf(" ⚠️ ERROR: %v", m.err)) + "\n\n")
	}
	if m.status != "" {
		s.WriteString(SuccessStyle().Render(" "+m.status) + "\n\n")
	}

	switch m.state {
	case vaultBrowse:
		s.WriteString(" 📂 VAULT REGISTRY\n")
		if len(m.items) == 0 {
			s.WriteString("   (Vault is empty)\n")
		}
		for i, item := range m.items {
			icon := "📄"
			if item.Type == "folder" {
				icon = "📁"
			} else if strings.HasPrefix(item.MimeType, "image/") || strings.HasSuffix(item.Name, ".png") || strings.HasSuffix(item.Name, ".jpg") {
				icon = "🖼️"
			}

			line := fmt.Sprintf("%d. %s %-30s %10s", i, icon, item.Name, formatSize(item.Size))
			style := MenuStyle(i == m.cursor)
			s.WriteString(style.Render(line) + "\n")
		}
		s.WriteString("\n" + MutedStyle().Render(" [ctrl+u]pload [ctrl+e]xport [ctrl+d]elete [ctrl+i]mport [ESC]Reset"))

	case vaultUpload:
		s.WriteString("📤 Upload File to Vault\n\n")
		s.WriteString("Enter path to file:\n")
		s.WriteString(m.input.View())
		s.WriteString("\n\n" + vaultHelpStyle.Render("[ENTER] Upload [ESC] Cancel"))

	case vaultExport:
		s.WriteString("📤 Exporting Package\n\n")
		s.WriteString("Enter share password to protect this package:\n")
		s.WriteString(m.input.View())
		s.WriteString("\n\n" + vaultHelpStyle.Render("[ENTER] Export [ESC] Cancel"))

	case vaultImport:
		if m.previewPkg == nil {
			s.WriteString("📥 Import Package\n\n")
			s.WriteString("Enter path to .nexusvault file:\n")
			s.WriteString(m.input.View())
			s.WriteString("\n\n" + vaultHelpStyle.Render("[ENTER] Preview [ESC] Cancel"))
		} else {
			s.WriteString("📦 Incoming Package — Review Before Import\n")
			s.WriteString(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(
				fmt.Sprintf("Name:     %s\nType:     %s\nSize:     %s  │  Files: %d\nKeywords: %s\nSender:   %s",
					m.previewPkg.Preview.Name,
					m.previewPkg.MimeType,
					formatSize(m.previewPkg.Preview.SizeBytes),
					m.previewPkg.Preview.FileCount,
					strings.Join(m.previewPkg.Preview.Keywords, ", "),
					m.renderVerification(m.previewPkg),
				)) + "\n\n")
			s.WriteString("Enter share password to decrypt:\n")
			s.WriteString(m.input.View())
			s.WriteString("\n\n" + vaultHelpStyle.Render("[ENTER] Import [ESC] Cancel"))
		}
	}

	return chatBoxStyle.Render(s.String())
}

func (m vaultModel) renderVerification(pkg *NexusVaultPackage) string {
	// Simple signature check logic
	manifest, _ := json.Marshal(pkg.Preview)
	signData := append(manifest, []byte(pkg.MimeType)...)
	valid, err := VerifyData(signData, pkg.Signature, pkg.SenderPubKey)
	if err == nil && valid {
		return "✅ Signature Verified"
	}
	return "⚠️ Unverified Sender"
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}
