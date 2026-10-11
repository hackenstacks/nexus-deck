package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// APIServer holds the running server state.
type APIServer struct {
	token string
	user  *User
	mux   *http.ServeMux
}

// NewAPIServer creates a new API server with a random Bearer token.
func NewAPIServer(user *User) *APIServer {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	s := &APIServer{token: token, user: user, mux: http.NewServeMux()}
	s.registerRoutes()
	return s
}

// Start begins listening on 127.0.0.1:8080.
func (s *APIServer) Start() error {
	fmt.Printf("\n🔑 API Token: %s\n", s.token)
	fmt.Printf("🌐 API listening on http://127.0.0.1:8080\n\n")
	return http.ListenAndServe("127.0.0.1:8080", s.mux)
}

// auth middleware — checks Bearer token.
func (s *APIServer) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") || strings.TrimPrefix(header, "Bearer ") != s.token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *APIServer) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (s *APIServer) registerRoutes() {
	s.mux.HandleFunc("GET /api/v1/characters",     s.auth(s.handleListCharacters))
	s.mux.HandleFunc("GET /api/v1/characters/{id}", s.auth(s.handleGetCharacter))
	s.mux.HandleFunc("GET /api/v1/sessions",        s.auth(s.handleListSessions))
	s.mux.HandleFunc("POST /api/v1/chat",           s.auth(s.handleChat))
	s.mux.HandleFunc("GET /api/v1/status",          s.auth(s.handleStatus))
}

func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.json(w, map[string]string{"status": "ok", "version": "1.0"})
}

func (s *APIServer) handleListCharacters(w http.ResponseWriter, r *http.Request) {
	chars, err := ListCharacters(s.user)
	if err != nil {
		http.Error(w, `{"error":"failed to list characters"}`, http.StatusInternalServerError)
		return
	}
	s.json(w, chars)
}

func (s *APIServer) handleGetCharacter(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	char, err := LoadCharacter(s.user, id)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	// Strip private key before returning
	char.PrivateKeyEncA = ""
	s.json(w, char)
}

func (s *APIServer) handleListSessions(w http.ResponseWriter, r *http.Request) {
	keys, err := ListEncryptedFiles(s.user, "chat_sessions")
	if err != nil {
		http.Error(w, `{"error":"failed to list sessions"}`, http.StatusInternalServerError)
		return
	}
	s.json(w, map[string]any{"sessions": keys, "count": len(keys)})
}

func (s *APIServer) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CharacterID string `json:"character_id"`
		Message     string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.CharacterID == "" || req.Message == "" {
		http.Error(w, `{"error":"character_id and message required"}`, http.StatusBadRequest)
		return
	}
	// For now return acknowledgement — full LLM integration is Phase K+
	s.json(w, map[string]string{
		"status":       "received",
		"character_id": req.CharacterID,
		"message":      req.Message,
	})
}
