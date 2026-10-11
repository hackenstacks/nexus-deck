package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const NexusAPIURL = "http://localhost:8801"

// --- API Data Structures (The Contract) ---

type NodeInfo struct {
	NodeID        string  `json:"node_id"`
	NexiumBalance float64 `json:"nexium_balance"`
	VestalStreak  int     `json:"vestal_streak"`
}

type ServiceStatus struct {
	Name   string `json:"name"`
	State  string `json:"state"` // "Running", "Stopped", "Error"
	Port   int    `json:"port"`
	Health string `json:"health"`
}

type GateStats struct {
	TorSpeed      string   `json:"tor_speed"`
	I2PSpeed      string   `json:"i2p_speed"`
	YggdrasilState string  `json:"yggdrasil_state"`
	RecentPermits  []string `json:"recent_permits"`
	Intercepted    int      `json:"intercepted_count"`
}

// --- Bubble Tea Messages ---

type APIUpdateMsg struct {
	Node  NodeInfo
	Pulse []ServiceStatus
	Gate  GateStats
	Err   error
}

// --- Fetcher Logic ---

// portMap maps container names (as returned by podman) to their service ports.
var portMap = map[string]int{
	"nexus-tor":       1080,
	"nexus-i2p":       4447,
	"nexus-yggdrasil": 9001,
	"nexus-reticulum": 4242,
	"nexus-dnscrypt":  5353,
	"nexus-dns":       5353,
	"nexus-batman":    0,
}

func FetchNexusData() tea.Msg {
	client := &http.Client{Timeout: 2 * time.Second}

	var msg APIUpdateMsg
	var fetchErr error

	// 1. GET /health → NodeInfo
	resp, err := client.Get(NexusAPIURL + "/health")
	if err == nil {
		var h struct {
			Status     string `json:"status"`
			Containers struct {
				Running int `json:"running"`
				Total   int `json:"total"`
			} `json:"containers"`
			Traffic struct {
				UptimeSeconds int64 `json:"uptime_seconds"`
			} `json:"traffic"`
		}
		if json.NewDecoder(resp.Body).Decode(&h) == nil {
			msg.Node.NodeID = "NXS-NODE"
			msg.Node.NexiumBalance = float64(h.Containers.Running)
			msg.Node.VestalStreak = int(h.Traffic.UptimeSeconds / 86400)
		}
		resp.Body.Close()
	} else {
		fetchErr = err
	}

	// 2. GET /health/containers → []ServiceStatus
	// Response shape: {"containers": [...], "summary": {...}}
	resp, err = client.Get(NexusAPIURL + "/health/containers")
	if err == nil {
		var payload struct {
			Containers []struct {
				Name    string `json:"name"`
				Running bool   `json:"running"`
				Health  string `json:"health"`
			} `json:"containers"`
		}
		if json.NewDecoder(resp.Body).Decode(&payload) == nil {
			for _, c := range payload.Containers {
				state := "Stopped"
				if c.Running {
					state = "Running"
				}
				msg.Pulse = append(msg.Pulse, ServiceStatus{
					Name:   c.Name,
					State:  state,
					Port:   portMap[c.Name],
					Health: c.Health,
				})
			}
		}
		resp.Body.Close()
	} else {
		fetchErr = err
	}

	// 3. GET /health/networks → GateStats
	// Response shape: {"tor":{"status":"up","latency_ms":42}, "i2p":{...}, ...}
	resp, err = client.Get(NexusAPIURL + "/health/networks")
	if err == nil {
		var networks map[string]struct {
			Status    string  `json:"status"`
			LatencyMs float64 `json:"latency_ms"`
		}
		if json.NewDecoder(resp.Body).Decode(&networks) == nil {
			if tor, ok := networks["tor"]; ok {
				msg.Gate.TorSpeed = fmt.Sprintf("%s %.0fms", tor.Status, tor.LatencyMs)
			}
			if i2p, ok := networks["i2p"]; ok {
				msg.Gate.I2PSpeed = fmt.Sprintf("%s %.0fms", i2p.Status, i2p.LatencyMs)
			}
			if ygg, ok := networks["yggdrasil"]; ok {
				msg.Gate.YggdrasilState = ygg.Status
			}
		}
		resp.Body.Close()
	} else {
		fetchErr = err
	}

	if fetchErr != nil && len(msg.Pulse) == 0 {
		msg.Err = fmt.Errorf("API offline (check localhost:8801)")
	}

	return msg
}

// HeartbeatTick triggers a refresh every 5 seconds
func HeartbeatTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return FetchNexusData()
	})
}
