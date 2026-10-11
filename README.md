# 🛡️ Military-Grade CLI Character Generator 🦾

This is a highly secure, CLI-first character generator application written in Go. It is designed for creating, managing, and interacting with AI characters in a secure, offline-first environment.

## ✨ Features

*   **🔒 Secure & Private:** All data is encrypted locally using military-grade encryption. No data is stored remotely by default.
*   **🤖 Multi-API Support:** Connect to a variety of LLM and image generation APIs, including:
    *   Free, public APIs (default)
    *   AI Horde
    *   Pollinations.ai
    *   ImageRouter
    *   Mistral
    *   Groq
    *   Hugging Face
    *   OpenAI
    *   Claude
    *   Custom user-defined APIs
*   **🎭 Character Card Compatibility:** Import and export character cards from popular formats like Silly Tavern and Open Character AI.
*   **📚 Local Library:** Store and manage your generated characters and files in a local, encrypted library.
*   **🖼️ Inline Image Generation:** Generate images within your chats based on the conversation.
*   **🧠 Advanced Memory System:**
    *   **Working/Short-Term Memory (STM):** For immediate task execution.
    *   **Episodic Memory:** Stores past experiences and session histories.
    *   **Semantic Memory:** A repository for factual knowledge and user profiles.
    *   **Procedural Memory:** Encodes rules, skills, and protocols.
*   **💬 Chat Summarization:** Conversations are automatically summarized to maintain context and relevance.
*   **💅 Beautiful TUI:** A beautiful and intuitive text-based user interface built with Bubble Tea and Lip Gloss.
*   **⚔️ Powerful Commands:** A rich set of slash commands to control the application.

## 🚀 Getting Started

1.  **Build the application:**
    ```bash
    go build .
    ```
2.  **Run the application:**
    ```bash
    ./char-gen-cli
    ```

## 📝 Commands

*   `/help`: Show a list of available commands.
*   `/summarize`: Summarize the current conversation.
*   `/character {name} {message}`: Send a message to a specific character.
*   `/save`: Save the current chat session.
*   `/narrator {prompt}`: Use a narrator to guide the story.
*   `/ai2ai {topic}`: Have two AIs converse on a given topic.
*   `/quit` or `/end`: Exit the application.
*   `/image {prompt}`: Generate an image.
*   `/review`: Review the chat back to the last summary.
*   `/system {prompt}`: Set a system prompt to steer the conversation.
*   `/dm {character} {message}`: Send a private message to a character.
*   `/invite {character}`: Invite a character to the current chat.
*   `/upload {path}`: Upload a file.
*   `/topic {message}`: Set or change the topic of conversation.
*   `/boot {character}`: Remove a character from the chat.
*   `/lore {entry}`: Add a lore book entry.
*   `/note {message}`: Add a special note.
