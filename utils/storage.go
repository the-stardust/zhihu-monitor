package utils

import (
	"encoding/json"
	"os"
	"sync"
)

type Storage struct {
	mu    sync.RWMutex
	items []string
	file  string
}

func NewStorage(file string) *Storage {
	s := &Storage{
		file:  file,
		items: []string{},
	}
	s.load()
	return s
}

func (s *Storage) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &s.items); err != nil {
		return
	}
}

func (s *Storage) save() {
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(s.file, data, 0644)
}

func (s *Storage) Contains(itemID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range s.items {
		if id == itemID {
			return true
		}
	}
	return false
}

func (s *Storage) Add(itemID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.items {
		if id == itemID {
			return
		}
	}
	s.items = append(s.items, itemID)
	s.save()
}