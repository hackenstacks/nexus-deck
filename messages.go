package main

// Messages that are used to communicate between different parts of the application.

// BackToMainAppMsg is a message to go back to the main application screen.
type BackToMainAppMsg struct{}

// ShowVaultMsg is a message to show the vault screen.
type ShowVaultMsg struct{}

// LoginSuccessMsg is a message to indicate a successful login.
type LoginSuccessMsg struct {
	User *User
}

// ShowCreateAccountMsg is a message to show the create account screen.
type ShowCreateAccountMsg struct{}

// AccountCreatedMsg is a message to indicate that an account has been created.
type AccountCreatedMsg struct{}

// ShowCharacterGeneratorMsg is a message to show the character generator screen.
type ShowCharacterGeneratorMsg struct{}

// ShowApiSettingsMsg is a message to show the API settings screen.
type ShowApiSettingsMsg struct{}

// ShowLibraryMsg is a message to show the library screen.
type ShowLibraryMsg struct{}

// ShowChatMsg is a message to show the chat screen.
type ShowChatMsg struct{}

// LogoutMsg is a message to log out the user.
type LogoutMsg struct{}

// RequestExecApprovalMsg is sent to trigger the terminal approval popup.
type RequestExecApprovalMsg struct {
	CharID  string
	Command string
}
