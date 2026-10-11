package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Entity represents a person, place, or thing in the knowledge graph.
type Entity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`        // person, place, object, concept
	Description string `json:"description"`
}

// Relation defines a directed connection between two entities.
type Relation struct {
	SubjectID   string `json:"subjectId"`
	Predicate   string `json:"predicate"` // "is a", "located in", "friends with"
	ObjectID    string `json:"objectId"`
	Strength    float64 `json:"strength"`  // 0.0 to 1.0
}

// KnowledgeGraph manages entities and their relationships.
type KnowledgeGraph struct {
	Entities  map[string]Entity   `json:"entities"`
	Relations []Relation          `json:"relations"`
}

func NewKnowledgeGraph() *KnowledgeGraph {
	return &KnowledgeGraph{
		Entities:  make(map[string]Entity),
		Relations: []Relation{},
	}
}

// AddEntity adds a new entity to the graph.
func (kg *KnowledgeGraph) AddEntity(e Entity) {
	kg.Entities[e.ID] = e
}

// AddRelation adds a relationship between entities.
func (kg *KnowledgeGraph) AddRelation(r Relation) {
	kg.Relations = append(kg.Relations, r)
}

// SaveKnowledgeGraph encrypts and saves the character's knowledge graph.
func SaveKnowledgeGraph(user *User, characterID string, kg *KnowledgeGraph) error {
	data, err := json.Marshal(kg)
	if err != nil {
		return err
	}
	return SaveEncryptedData(user, user.Username, "knowledge", characterID, data)
}

// LoadKnowledgeGraph loads and decrypts the character's knowledge graph.
func LoadKnowledgeGraph(user *User, characterID string) (*KnowledgeGraph, error) {
	data, err := LoadEncryptedData(user, user.Username, "knowledge", characterID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return NewKnowledgeGraph(), nil
		}
		return nil, err
	}
	var kg KnowledgeGraph
	err = json.Unmarshal(data, &kg)
	return &kg, err
}

// QueryKnowledge retrieves entities and relations related to a search term.
func (kg *KnowledgeGraph) QueryKnowledge(term string) string {
	var sb strings.Builder
	term = strings.ToLower(term)
	
	sb.WriteString("\n--- Knowledge Graph Context ---\n")
	found := false
	
	for _, entity := range kg.Entities {
		if strings.Contains(strings.ToLower(entity.Name), term) || strings.Contains(strings.ToLower(entity.Description), term) {
			sb.WriteString(fmt.Sprintf("Entity: %s (%s) - %s\n", entity.Name, entity.Type, entity.Description))
			found = true
			
			// Find related entities
			for _, rel := range kg.Relations {
				if rel.SubjectID == entity.ID {
					obj := kg.Entities[rel.ObjectID]
					sb.WriteString(fmt.Sprintf("  - %s %s %s\n", entity.Name, rel.Predicate, obj.Name))
				}
			}
		}
	}
	
	if !found {
		return ""
	}
	return sb.String()
}
