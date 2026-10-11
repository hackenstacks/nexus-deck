package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type HydraStatus struct {
	Profile    string
	Tier       string
	Containers []HydraContainer
	Ports      map[string]bool
	GhostGate  bool
	LastPoll   time.Time
}

type HydraContainer struct {
	Name   string
	Status string
}

// DeckTab represents a tab in the MNEMOSYNE Deck.
type DeckTab int

const (
	TabPulse   DeckTab = iota // 📈 Node Pulse / Audit
	TabSanctum                // 🤖 Character Chat
	TabStudio                 // 🃏 Character Studio
	TabVault                  // 🏛️ Sovereign Vault
	TabGate                   // 👻 Ghost Gate HUD
	TabSettings               // ⚙️ API Settings
	TabLogs                   // 📋 Live Log Viewer
	TabHydra                  // 🐍 Hydra Darknet HUD
	TabSnitch                 // 👁  OpenSnitch Monitor
)

// DeckModel is the master model for the unified NeXuS Cockpit.
type DeckModel struct {
	activeTab DeckTab
	width     int
	height    int

	// Sub-models (wrapping existing logic)
	chatModel     chatModel
	studioModel   characterGeneratorModel
	vaultModel    vaultModel
	settingsModel apiSettingsModel

	// Init State
	initialized bool
	initStep    string
	user        *User

	// Node Status
	nodeID        string
	vestalStreak  int
	nexiumBalance float64

	// Live API Data
	pulseData []ServiceStatus
	gateData  GateStats
	gateState GateState
	hydra     HydraStatus

	// Hydra tab cursor
	hydraSelected int

	// OpenSnitch tab
	snitchEvents    []SnitchEvent
	snitchSelected  int
	snitchActive    bool
	snitchRulesView bool       // toggle between events and rules list
	snitchMode      SnitchMode // current operating mode
	snitchPending   *SnitchPending // connection waiting for verdict (Interactive)
	snitchHistView  bool       // toggle history view

	// UI Elements
	viewport viewport.Model
	alerts   []string
}

func NewDeckModel(user *User) DeckModel {
	return DeckModel{
		activeTab:   TabSanctum,
		user:        user,
		initialized: false,
		initStep:    "Standby...",
		nodeID:      "NXS-BOOTING",
		alerts:      []string{"System handoff from Login Shield..."},
	}
}

// Init Messages
type InitStepMsg string
type InitCompleteMsg struct {
	Chat   chatModel
	Studio characterGeneratorModel
	Vault  vaultModel
	Settings apiSettingsModel
}

func (m DeckModel) Init() tea.Cmd {
	return func() tea.Msg {
		NexusLog("INFO", "deck", "Starting initialization sequence")
		return InitStepMsg("Syncing API Configurations...")
	}
}

func (m DeckModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case InitStepMsg:
		m.initStep = string(msg)
		step := string(msg)
		NexusLog("INFO", "deck", "Init step: "+step)
		
		return m, func() tea.Msg {
			time.Sleep(100 * time.Millisecond)
			switch step {
			case "Syncing API Configurations...":
				return InitStepMsg("Forging AI Neural Links...")
			case "Forging AI Neural Links...":
				return InitStepMsg("Accessing Encrypted Vault...")
			case "Accessing Encrypted Vault...":
				return m.performStepInit()
			}
			return nil
		}

	case InitCompleteMsg:
		m.chatModel = msg.Chat
		m.studioModel = msg.Studio
		m.vaultModel = msg.Vault
		m.settingsModel = msg.Settings
		// Carry current terminal dimensions into freshly initialized sub-models
		if m.width > 0 {
			sized := tea.WindowSizeMsg{Width: m.width, Height: m.height}
			if nc, _ := m.chatModel.Update(sized); nc != nil { m.chatModel = nc.(chatModel) }
		}
		m.initialized = true
		m.nodeID = "NXS-READY"
		NexusLog("INFO", "deck", "Initialization complete")
		return m, tea.Batch(
			m.chatModel.Init(),
			m.studioModel.Init(),
			m.vaultModel.Init(),
			func() tea.Msg { return FetchNexusData() },
			HeartbeatTick(),
			HydraHeartbeat(),
			SnitchHeartbeat(),
		)

	case ProviderChangedMsg:
		// User saved a new LLM provider in Settings — reinit the live chat service immediately.
		newSvc, err := LLMFactory(msg.Provider, msg.APIConfig)
		if err != nil {
			NexusLog("WARN", "deck", "ProviderChanged: LLMFactory failed for "+string(msg.Provider)+": "+err.Error())
		} else {
			m.chatModel.llmService = newSvc
			m.chatModel.selectedLLMProvider = msg.Provider
			m.chatModel.apiConfig = &msg.APIConfig
			// Auto-select the default model for the new provider so chat works right away
			m.chatModel.selectedLLMModel = getDefaultModelForProvider(&msg.APIConfig)
			NexusLog("INFO", "deck", "LLM switched to "+string(msg.Provider)+" model="+m.chatModel.selectedLLMModel)
		}
		return m, nil

	case ModelChangedMsg:
		// User picked a new model in Settings — update the live chat service immediately.
		m.chatModel.apiConfig = &msg.APIConfig
		m.chatModel.selectedLLMModel = getDefaultModelForProvider(&msg.APIConfig)
		NexusLog("INFO", "deck", "Model switched to "+string(msg.Model)+" on "+string(msg.Provider)+" selectedLLMModel="+m.chatModel.selectedLLMModel)
		return m, nil

	case APIUpdateMsg:
		if msg.Err == nil {
			m.nodeID = msg.Node.NodeID
			m.nexiumBalance = msg.Node.NexiumBalance
			m.vestalStreak = msg.Node.VestalStreak
			m.pulseData = msg.Pulse
			m.gateData = msg.Gate
		} else {
			m.alerts = append(m.alerts, msg.Err.Error())
		}
		return m, HeartbeatTick()

	case HydraTickMsg:
		m.hydra = msg.Status
		// Clamp cursor in case items disappeared
		if items := m.hydra.AllItems(); m.hydraSelected >= len(items) && len(items) > 0 {
			m.hydraSelected = len(items) - 1
		}
		return m, HydraHeartbeat()

	case ContainerActionMsg:
		if msg.Err != nil {
			m.alerts = append(m.alerts, fmt.Sprintf("%s %s failed: %v", msg.Action, msg.Name, msg.Err))
		}
		// Immediate re-poll to reflect new state
		return m, func() tea.Msg { return HydraTickMsg{Status: pollHydraStatus()} }

	case SnitchPendingMsg:
		m.snitchPending = msg.Pending
		// Re-subscribe for next pending connection + start countdown
		return m, tea.Batch(SnitchWatchPendingCmd(), SnitchCountdownCmd())

	case SnitchVerdictSentMsg:
		if m.snitchPending != nil && m.snitchPending.ID == msg.ID {
			m.snitchPending = nil
		}
		m.alerts = append(m.alerts, fmt.Sprintf("snitch: %s — %s", msg.Action, msg.ID[:8]))
		// Re-subscribe for next pending
		return m, SnitchWatchPendingCmd()

	case SnitchModeChangedMsg:
		m.snitchMode = msg.Mode
		m.snitchPending = nil
		if msg.Mode == ModeInteractive {
			return m, SnitchWatchPendingCmd()
		}
		return m, nil

	case SnitchCountdownTickMsg:
		// Keep ticking while a connection is pending
		if m.snitchPending != nil && m.snitchPending.Remaining() > 0 {
			return m, SnitchCountdownCmd()
		}
		if m.snitchPending != nil {
			m.snitchPending = nil // timed out
		}
		return m, nil

	case SnitchUpdateMsg:
		m.snitchActive = msg.Active
		m.snitchEvents = msg.Events
		if m.snitchSelected >= len(m.snitchEvents) && len(m.snitchEvents) > 0 {
			m.snitchSelected = len(m.snitchEvents) - 1
		}
		return m, SnitchHeartbeat()

	case SnitchRuleWrittenMsg:
		if msg.Err != nil {
			m.alerts = append(m.alerts, fmt.Sprintf("snitch rule failed: %v", msg.Err))
		} else {
			m.alerts = append(m.alerts, fmt.Sprintf("rule saved: %s %s", msg.Action, msg.Rule.Name))
		}
		return m, nil

	case SnitchDaemonMsg:
		if msg.Err != nil {
			m.alerts = append(m.alerts, fmt.Sprintf("snitch %s failed: %v", msg.Action, msg.Err))
		} else {
			m.alerts = append(m.alerts, fmt.Sprintf("opensnitchd %sd", msg.Action))
		}
		return m, func() tea.Msg { return fetchSnitchData() }

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		newChat, _ := m.chatModel.Update(msg)
		m.chatModel = newChat.(chatModel)
		newStudio, _ := m.studioModel.Update(msg)
		m.studioModel = newStudio.(characterGeneratorModel)
		newVault, _ := m.vaultModel.Update(msg)
		m.vaultModel = newVault.(vaultModel)
		newSettings, _ := m.settingsModel.Update(msg)
		m.settingsModel = newSettings.(apiSettingsModel)

	case tea.KeyMsg:
		if m.activeTab == TabGate {
			if m.gateState.cmdFocused {
				switch msg.String() {
				case "esc":
					m.gateState.cmdFocused = false
					return m, nil
				case "enter":
					m.gateState.cmdResult = "Command: " + m.gateState.cmdInput
					m.gateState.cmdResultTime = time.Now()
					m.gateState.cmdInput = ""
					m.gateState.cmdFocused = false
					return m, nil
				case "backspace":
					if len(m.gateState.cmdInput) > 0 {
						m.gateState.cmdInput = m.gateState.cmdInput[:len(m.gateState.cmdInput)-1]
					}
					return m, nil
				default:
					if len(msg.String()) == 1 {
						m.gateState.cmdInput += msg.String()
					}
					return m, nil
				}
			} else if msg.String() == "ctrl+i" {
				m.gateState.cmdFocused = true
				return m, nil
			}
		}

		// OpenSnitch tab navigation + allow/deny
		if m.activeTab == TabSnitch {
			switch msg.String() {
			case "up", "k":
				if m.snitchSelected > 0 {
					m.snitchSelected--
				}
				return m, nil
			case "down", "j":
				if m.snitchSelected < len(m.snitchEvents)-1 {
					m.snitchSelected++
				}
				return m, nil
			case "ctrl+a":
				if !m.snitchRulesView && len(m.snitchEvents) > 0 && m.snitchSelected < len(m.snitchEvents) {
					return m, SnitchAllowCmd(m.snitchEvents[m.snitchSelected])
				}
				return m, nil
			case "ctrl+d":
				if !m.snitchRulesView && len(m.snitchEvents) > 0 && m.snitchSelected < len(m.snitchEvents) {
					return m, SnitchDenyCmd(m.snitchEvents[m.snitchSelected])
				}
				return m, nil
			case "ctrl+r":
				m.snitchRulesView = !m.snitchRulesView
				m.snitchSelected = 0
				return m, nil
			case "ctrl+R":
				return m, SnitchReportCmd()
			case "ctrl+s":
				if !m.snitchActive {
					return m, SnitchStartCmd()
				}
				return m, nil
			case "ctrl+x":
				if m.snitchActive {
					return m, SnitchStopCmd()
				}
				return m, nil
			case "ctrl+e":
				if len(m.snitchEvents) > 0 {
					return m, SnitchExportCmd(m.snitchEvents)
				}
				return m, nil
			}
		}

		// Hydra tab navigation + service control
		if m.activeTab == TabHydra {
			items := m.hydra.AllItems()
			switch msg.String() {
			case "up", "k":
				if m.hydraSelected > 0 {
					m.hydraSelected--
				}
				return m, nil
			case "down", "j":
				if m.hydraSelected < len(items)-1 {
					m.hydraSelected++
				}
				return m, nil
			case "ctrl+s":
				if len(items) > 0 && m.hydraSelected < len(items) {
					return m, HydraItemStartCmd(items[m.hydraSelected])
				}
				return m, nil
			case "ctrl+x":
				if len(items) > 0 && m.hydraSelected < len(items) {
					return m, HydraItemStopCmd(items[m.hydraSelected])
				}
				return m, nil
			}
		}

		// inMenuTab = any tab with its own interactive UI that needs all keys
		inMenuTab := m.activeTab == TabSettings || m.activeTab == TabStudio ||
			m.activeTab == TabVault || m.activeTab == TabLogs

		// Only ctrl+c/ctrl+q are truly global — everything else respects menu context
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit
		}

		// F-keys always switch tabs — even from inside menus.
		switch msg.String() {
		case "f1":
			m.activeTab = TabPulse
			return m, nil
		case "f2":
			m.activeTab = TabSanctum
			return m, nil
		case "f3":
			m.activeTab = TabStudio
			return m, nil
		case "f4":
			m.activeTab = TabVault
			return m, nil
		case "f5":
			m.activeTab = TabGate
			return m, nil
		case "f6":
			m.activeTab = TabLogs
			return m, nil
		case "f7":
			m.activeTab = TabHydra
			return m, nil
		case "f8":
			m.activeTab = TabSnitch
			return m, nil
		case "ctrl+s":
			// Only navigate TO settings if not already in a menu tab that owns ctrl+s
			if m.activeTab != TabHydra && m.activeTab != TabSnitch && !inMenuTab {
				m.activeTab = TabSettings
				return m, nil
			}
		}

		// All remaining shortcuts ONLY fire when NOT inside a menu tab.
		// Inside Settings/Studio/Vault/Logs the sub-model owns the keyboard.
		if !inMenuTab {
			chatInputFocused := m.initialized && m.activeTab == TabSanctum && m.chatModel.textInput.Focused()
			if !chatInputFocused {
				switch msg.String() {
				case "ctrl+1":
					m.activeTab = TabPulse
					return m, nil
				case "ctrl+2":
					m.activeTab = TabSanctum
					return m, nil
				case "ctrl+3":
					m.activeTab = TabStudio
					return m, nil
				case "ctrl+4":
					m.activeTab = TabVault
					return m, nil
				case "ctrl+5":
					m.activeTab = TabGate
					return m, nil
				case "ctrl+6":
					m.activeTab = TabLogs
					return m, nil
				case "ctrl+7":
					m.activeTab = TabHydra
					return m, nil
				case "ctrl+8":
					m.activeTab = TabSnitch
					return m, nil
				case "ctrl+t": // Cycle themes
					themes := []string{"fire", "matrix", "tron", "dark"}
					curr := ActiveTheme.Name
					for i, t := range themes {
						if t == curr {
							SetTheme(themes[(i+1)%len(themes)])
							break
						}
					}
					return m, nil
				case "ctrl+g": // Toggle glow
					SetGlow(!ActiveTheme.Glow)
					return m, nil
				case "ctrl+o": // Open last image in external viewer
					return m, openLastImageCmd(m.chatModel.messages, m.user)
				}
			}
		}
	}

	if m.initialized {
		var cmd tea.Cmd
		switch m.activeTab {
		case TabSanctum:
			newChat, c := m.chatModel.Update(msg)
			m.chatModel = newChat.(chatModel)
			cmd = c
		case TabStudio:
			newStudio, c := m.studioModel.Update(msg)
			m.studioModel = newStudio.(characterGeneratorModel)
			cmd = c
		case TabVault:
			newVault, c := m.vaultModel.Update(msg)
			m.vaultModel = newVault.(vaultModel)
			cmd = c
		case TabSettings:
			newSettings, c := m.settingsModel.Update(msg)
			m.settingsModel = newSettings.(apiSettingsModel)
			cmd = c
		}
		return m, cmd
	}

	return m, nil
}

func (m DeckModel) performStepInit() tea.Msg {
	apiCfg, _ := LoadAPIConfig(m.user)
	// NexusApiRouter — self-healing multi-provider routing
	// Automatically discovers all enabled/keyed providers and builds
	// text/vision/RAG chains with score-based failover.
	router := NewNexusApiRouter(&apiCfg)

	// Fallback single LLM for legacy paths that still need one (summarizer, etc.)
	llmSvc, err := LLMFactory(apiCfg.SelectedLLMProvider, apiCfg)
	if err != nil {
		NexusLog("WARN", "deck", "primary LLM init failed ("+err.Error()+"), falling back to Pollinations")
		apiCfg.SelectedLLMProvider = ProviderPollinations
		llmSvc, _ = LLMFactory(ProviderPollinations, apiCfg)
	}

	imgSvc, err := ImageFactory(apiCfg.SelectedImageProvider, apiCfg)
	if err != nil {
		NexusLog("WARN", "deck", "Image init failed ("+err.Error()+"), falling back to Pollinations")
		apiCfg.SelectedImageProvider = ProviderPollinations
		imgSvc, _ = ImageFactory(ProviderPollinations, apiCfg)
	}
	embSvc, _ := EmbeddingFactory(apiCfg.SelectedEmbeddingProvider, apiCfg)
	summarizer, _ := SummarizerFactory(apiCfg.SelectedRAGLLMProvider, apiCfg)
	mm := NewLocalMemoryManager(embSvc, summarizer)

	_ = llmSvc // router is primary; llmSvc kept for fallback paths

	return InitCompleteMsg{
		Chat:   initialChatModel(m.user, &apiCfg, mm, router, router, imgSvc),
		Studio: initialCharacterGeneratorModel(m.user),
		Vault:  initialVaultModel(m.user),
		Settings: initialApiSettingsModel(m.user),
	}
}

func (m DeckModel) View() string {
	if !m.initialized {
		return m.renderHeader() + "\n\n" + 
			SuccessStyle().Render(" 📡 INITIALIZING NEURAL LINKS... ") + "\n" +
			lipgloss.NewStyle().Foreground(ActiveTheme.Accent).Render(" STEP: "+m.initStep) + "\n" +
			MutedStyle().Render(" Unlocking sovereign context...") + "\n\n" +
			m.renderFooter()
	}

	var sb strings.Builder

	// 1. Header
	header := m.renderHeader()
	sb.WriteString(header + "\n")

	// 2. Tab Bar
	tabs := m.renderTabs()
	sb.WriteString(tabs + "\n\n")

	// 3. Main Content Area
	var content string
	switch m.activeTab {
	case TabPulse:
		content = m.renderPulseTab()
	case TabSanctum:
		content = m.chatModel.View()
	case TabStudio:
		content = m.studioModel.View()
	case TabVault:
		content = m.vaultModel.View()
	case TabGate:
		content = renderGateHUD(m.gateState, m.gateData, m.width)
	case TabSettings:
		content = m.settingsModel.View()
	case TabLogs:
		content = m.renderLogsTab()
	case TabHydra:
		content = renderHydraTab(m.hydra, m.width, m.hydraSelected)
	case TabSnitch:
		content = m.renderSnitchTab()
	}

	// Height calculation for the body
	contentHeight := m.height - 10
	var mainContent string

	if m.width >= 120 {
		sideWidth := 22
		mainWidth := m.width - sideWidth - 8
		sidebar := renderHydraSidebar(m.hydra, contentHeight)
		mainContent = lipgloss.JoinHorizontal(
			lipgloss.Top,
			lipgloss.NewStyle().Height(contentHeight).Width(mainWidth).Render(content),
			lipgloss.NewStyle().Height(contentHeight).Width(sideWidth).Render(sidebar),
		)
	} else {
		mainContent = lipgloss.NewStyle().
			Height(contentHeight).
			Width(m.width - 4).
			Render(content)
	}
	
	sb.WriteString(BorderedBox(true).Render(mainContent) + "\n")

	// 4. Footer
	footer := m.renderFooter()
	sb.WriteString(footer)

	return sb.String()
}

func (m DeckModel) renderHeader() string {
	title := TitleStyle().Render(" 🛰️  MNEMOSYNE DECK v1.0 ")
	node := MutedStyle().Render(fmt.Sprintf(" NODE: %s ", m.nodeID))
	streak := SuccessStyle().Render(fmt.Sprintf(" ⏳ %dd Streak ", m.vestalStreak))
	balance := lipgloss.NewStyle().Foreground(ActiveTheme.Accent).Render(fmt.Sprintf(" 💎 %.2f NeXiuM ", m.nexiumBalance))
	
	filler := strings.Repeat(" ", max(0, m.width-lipgloss.Width(title+node+streak+balance)-2))
	
	return lipgloss.JoinHorizontal(lipgloss.Top, title, node, filler, streak, balance)
}

func (m DeckModel) renderTabs() string {
	tabNames := []string{"^1. PULSE", "^2. SANCTUM", "^3. STUDIO", "^4. VAULT", "^5. GATE", "^S. SETTINGS", "F6. LOGS", "F7. HYDRA", "F8. SNITCH"}
	var renderedTabs []string

	for i, name := range tabNames {
		isActive := int(m.activeTab) == i
		if i == 5 && m.activeTab == TabSettings { isActive = true }
		if i == 6 && m.activeTab == TabLogs { isActive = true }
		if i == 7 && m.activeTab == TabHydra { isActive = true }
		if i == 8 && m.activeTab == TabSnitch { isActive = true }
		style := MenuStyle(isActive)
		renderedTabs = append(renderedTabs, style.Render(name))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
}

func (m DeckModel) renderFooter() string {
	var hint string
	switch m.activeTab {
	case TabHydra:
		hint = MutedStyle().Render(" [↑/↓] Select | [ctrl+s] Start | [ctrl+x] Stop | F7 Hydra | ctrl+q Quit ")
	case TabSnitch:
		if m.snitchRulesView {
			hint = MutedStyle().Render(" [ctrl+r] Live Feed | [↑/↓] Scroll | F8 Snitch | ctrl+q Quit ")
		} else {
			hint = MutedStyle().Render(" [ctrl+s] Start | [ctrl+x] Stop | [ctrl+e] Export | [↑/↓] Select | [ctrl+a] Allow | [ctrl+d] Deny | [ctrl+r] Rules | ctrl+q Quit ")
		}

	default:
		hint = MutedStyle().Render(" [F1-F5] Tabs | F6 Logs | F7 Hydra | F8 Snitch | ctrl+s Settings | ctrl+t Theme | ctrl+q Quit ")
	}
	alert := WarningStyle().Render(" 🔥 ALERT: " + m.alerts[len(m.alerts)-1])
	
	filler := strings.Repeat(" ", max(0, m.width-lipgloss.Width(hint+alert)-2))
	return lipgloss.JoinHorizontal(lipgloss.Bottom, hint, filler, alert)
}

func (m DeckModel) renderPulseTab() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle().Render(" 📈 NODE PULSE — SERVICE AUDIT ") + "\n\n")
	
	if len(m.pulseData) == 0 {
		sb.WriteString(MutedStyle().Render(" (No service data available from orchestrator)"))
		return sb.String()
	}

	for _, s := range m.pulseData {
		statusIcon := "🟢"
		if s.State != "Running" { statusIcon = "🔴" }
		
		line := fmt.Sprintf(" %s %-15s | Port: %-5d | State: %-10s | Health: %s", 
			statusIcon, s.Name, s.Port, s.State, s.Health)
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

func (m DeckModel) renderLogsTab() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle().Render(" 📋 NEXUS LIVE LOGS ") + "\n")
	sb.WriteString(MutedStyle().Render(fmt.Sprintf(" %s ", nexusLogPath)) + "\n\n")

	lines := GetRecentLogs(200)
	if len(lines) == 0 {
		sb.WriteString(MutedStyle().Render(" No log entries yet."))
		return sb.String()
	}

	errorStyle  := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444"))
	warnStyle   := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C"))
	infoStyle   := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))
	debugStyle  := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4"))

	// Show most recent lines — limited by content area height
	maxLines := m.height - 14
	if maxLines < 5 { maxLines = 5 }
	start := 0
	if len(lines) > maxLines { start = len(lines) - maxLines }

	for _, line := range lines[start:] {
		switch {
		case strings.Contains(line, " ERROR "):
			sb.WriteString(errorStyle.Render(line) + "\n")
		case strings.Contains(line, " WARN  "):
			sb.WriteString(warnStyle.Render(line) + "\n")
		case strings.Contains(line, " DEBUG "):
			sb.WriteString(debugStyle.Render(line) + "\n")
		default:
			sb.WriteString(infoStyle.Render(line) + "\n")
		}
	}
	return sb.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

