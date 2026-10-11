package main

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func setupAPISettingsTest(t *testing.T) {
	if err := os.RemoveAll("users"); err != nil {
		t.Fatalf("failed to remove users directory: %v", err)
	}
}

func TestAPISettings(t *testing.T) {
	setupAPISettingsTest(t)
	// 1. Create a new user with a username and password.
	username := "testuser"
	password := "password"
	user := &User{Username: username}

	// 2. Call SaveUser to save the user and generate the encryption key.
	if err := SaveUser(user, password); err != nil {
		t.Fatalf("SaveUser failed: %v", err)
	}

	// 3. Load the user using LoadUser to get the user with the encryption key.
	loadedUser, err := LoadUser(username, password)
	if err != nil {
		t.Fatalf("LoadUser failed: %v", err)
	}

	// Initialize the apiSettingsModel with the loaded user
	m := initialApiSettingsModel(loadedUser)

	// 1. Simulate the user selecting a provider.
	// For this test, we'll just set the selectedProvider directly.
	m.selectedProvider = ProviderMistral

	// 2. Simulate the user pressing ctrl+s to go to the setProviderDefaultView.
	m.currentView = providerDetailsView // Start in details view to allow ctrl+s
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = newModel.(apiSettingsModel)


	// Assert that the state has changed to setProviderDefaultView
	if m.currentView != setProviderDefaultView {
		t.Errorf("expected currentView to be %v, but got %v", setProviderDefaultView, m.currentView)
	}

	// 3. Simulate the user pressing '1' to set the provider as the default LLM provider.
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = newModel.(apiSettingsModel)

	// 4. Verify that the apiConfig.SelectedLLMProvider has been updated correctly.
	if m.apiConfig.SelectedLLMProvider != ProviderMistral {
		t.Errorf("expected SelectedLLMProvider to be %v, but got %v", ProviderMistral, m.apiConfig.SelectedLLMProvider)
	}

	// Test Case 2: Verify model selection functionality
	// Re-initialize the apiSettingsModel with the loaded user to start fresh
	m = initialApiSettingsModel(loadedUser)
	m.selectedProvider = ProviderMistral
	m.apiConfig.Mistral.APIKey = "mock-key" // Use mock API key

	// Simulate pressing alt+l to go to the modelSelectionView for LLM
	m.currentView = providerDetailsView // Start in details view to allow alt+l
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alt+l")})
	m = newModel.(apiSettingsModel)

	// Assert that the state has changed to modelSelectionView and currentModelListType is "llm"
	if m.currentView != modelSelectionView {
		t.Errorf("expected currentView to be %v, but got %v", modelSelectionView, m.currentView)
	}
	if m.currentModelListType != "llm" {
		t.Errorf("expected currentModelListType to be 'llm', but got '%s'", m.currentModelListType)
	}

	// Assume there's at least one model in the list for Mistral
	if len(m.llmModelsList.Items()) == 0 {
		t.Fatalf("expected Mistral LLM models list to not be empty")
	}

	// Simulate selecting the first model (assuming "mistral-tiny" is at index 0 or similar)
	m.llmModelsList.Select(0)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(apiSettingsModel)

	// Verify that the apiConfig.Mistral.DefaultLLMModel has been updated correctly.
	// This assumes that the first model in the mock list is "mock-llm-fast" or similar
	// For real Mistral, it would be "mistral-tiny" as per services.go fallback.
	expectedModel := LLMModel(m.llmModelsList.Items()[0].(providerItem).title)
	if m.apiConfig.Mistral.DefaultLLMModel != expectedModel {
		t.Errorf("expected Mistral DefaultLLMModel to be '%s', but got '%s'", expectedModel, m.apiConfig.Mistral.DefaultLLMModel)
	}

	// Assert that the state has returned to providerDetailsView
	if m.currentView != providerDetailsView {
		t.Errorf("expected currentView to be %v after model selection, but got %v", providerDetailsView, m.currentView)
	}
}
