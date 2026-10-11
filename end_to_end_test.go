package main

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestEndToEndFlow tests the complete application flow from login to character creation
func TestEndToEndFlow(t *testing.T) {
	// Clean up any existing test data
	if err := os.RemoveAll("users"); err != nil {
		t.Fatalf("failed to remove users directory: %v", err)
	}

	// Test data
	username := "testuser"
	password := "testpassword123"

	// Step 1: Test account creation
	t.Run("AccountCreation", func(t *testing.T) {
		// Initialize create account model
		createAccModel := initialCreateAccountModel()
		
		// Set username
		createAccModel.usernameInput.SetValue(username)
		
		// Set password
		createAccModel.passwordInput.SetValue(password)
		
		// Set confirm password
		createAccModel.confirmPasswordInput.SetValue(password)
		
		// Focus on confirm password field (last field)
		createAccModel.usernameInput.Blur()
		createAccModel.passwordInput.Blur()
		createAccModel.confirmPasswordInput.Focus()
		
		// Simulate pressing enter on confirm password field
		_, cmd := createAccModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Error("expected AccountCreatedMsg command after account creation")
		}
		
		// Check that user was created
		if !UserExists(username) {
			t.Error("user was not created successfully")
		}
	})

	// Step 2: Test login
	t.Run("Login", func(t *testing.T) {
		// Initialize login model
		loginModel := initialLoginModel()
		
		// Set username
		loginModel.inputs[0].SetValue(username)
		
		// Set password
		loginModel.inputs[1].SetValue(password)
		
		// Focus on submit button (index 2)
		loginModel.focusIndex = 2
		
		// Simulate pressing enter on submit button
		_, cmd := loginModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Error("expected LoginSuccessMsg command after login")
		}
		
		// Execute the command to get the user
		msg := cmd()
		loginSuccessMsg, ok := msg.(LoginSuccessMsg)
		if !ok {
			t.Error("expected LoginSuccessMsg but got different message type")
		}
		
		// Verify user was loaded correctly
		if loginSuccessMsg.User.Username != username {
			t.Errorf("expected username %s but got %s", username, loginSuccessMsg.User.Username)
		}
		
		// Verify encryption key was set
		if len(loginSuccessMsg.User.EncryptionKey) == 0 {
			t.Error("encryption key was not set for the user")
		}
	})

	// Step 3: Test basic API settings setup
	t.Run("APISettings", func(t *testing.T) {
		// Load the user we just created
		user, err := LoadUser(username, password)
		if err != nil {
			t.Fatalf("failed to load user: %v", err)
		}
		
		// Initialize API settings model - this tests that the model can be created
		apiSettingsModel := initialApiSettingsModel(user)
		
		// Verify the model was created successfully
		if apiSettingsModel.user != user {
			t.Error("API settings model was not initialized with correct user")
		}
		
		// Verify default API config was loaded (just check that it's not empty)
		if apiSettingsModel.apiConfig.SelectedLLMProvider == "" {
			t.Error("API config was not initialized properly")
		}
	})

	// Step 4: Test character generation
	t.Run("CharacterGeneration", func(t *testing.T) {
		// Load the user
		user, err := LoadUser(username, password)
		if err != nil {
			t.Fatalf("failed to load user: %v", err)
		}
		
		// Initialize character generator model
		charGenModel := initialCharacterGeneratorModel(user)
		
		// Switch to create view
		charGenModel.state = charGenCreateView
		
		// Set character name
		charGenModel.nameInput.SetValue("Test Character")
		
		// Set character description
		charGenModel.descriptionInput.SetValue("A brave warrior from the north")
		
		// Set personality
		charGenModel.personalityInput.SetValue("Brave, loyal, and strong")
		
		// Focus on first message input (index 4)
		charGenModel.focusedInput = 4
		
		// Simulate pressing enter to save character
		newModel, _ := charGenModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
		charGenModel = newModel.(characterGeneratorModel)
		
		// Check that character was saved (no command is generated, it saves directly)
		if charGenModel.saveSuccess != true {
			t.Error("expected character to be saved successfully")
		}
		
		// Note: We can't fully test the async generation here without a mock LLM service,
		// but we can verify the setup is correct
	})

	// Clean up
	if err := os.RemoveAll("users"); err != nil {
		t.Logf("warning: failed to clean up users directory: %v", err)
	}
}