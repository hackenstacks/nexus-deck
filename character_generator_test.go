package main

import (
	"os"
	"testing"
)

func setup(t *testing.T) {
	if err := os.RemoveAll("storage"); err != nil {
		t.Fatalf("failed to remove storage directory: %v", err)
	}
	if err := os.RemoveAll("users"); err != nil {
		t.Fatalf("failed to remove users directory: %v", err)
	}
}

func TestCharacterGenerator(t *testing.T) {
	setup(t)


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



	// 4. Create a new character struct.

	char := NewCharacter()

	char.Name = "Test Character"



	// 5. Call SaveCharacter to save the character.

	if err := SaveCharacter(loadedUser, char); err != nil {

		t.Fatalf("SaveCharacter failed: %v", err)

	}



	// 6. Call loadCharacterItems to get the list of characters.

	m := initialCharacterGeneratorModel(loadedUser)

	items := m.loadCharacterItems()



	// 7. Assert that the newly created character is in the list.

	found := false

	for _, item := range items {

		if item.FilterValue() == "Test Character" {

			found = true

			break

		}

	}

	if !found {

		t.Errorf("expected character 'Test Character' to be created, but it was not found")

	}

}




