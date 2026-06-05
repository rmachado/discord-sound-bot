package sound

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"sync"
)

type SoundEntry struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	File    string `json:"file"`
	Start   string `json:"start,omitempty"`
	End     string `json:"end,omitempty"`
	AddedAt string `json:"added_at"`
}

type registryData struct {
	Sounds map[string]*SoundEntry `json:"sounds"`
}

type Registry struct {
	entries map[string]*SoundEntry
	mu      sync.RWMutex
	path    string
}

func NewRegistry(path string) (*Registry, error) {
	r := &Registry{
		entries: make(map[string]*SoundEntry),
		path:    path,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, err
	}
	var reg registryData
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, err
	}
	if reg.Sounds != nil {
		r.entries = reg.Sounds
	}
	return r, nil
}

func (r *Registry) save() error {
	data, err := json.MarshalIndent(registryData{Sounds: r.entries}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0644)
}

func (r *Registry) Add(entry *SoundEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[entry.Name]; exists {
		r.entries[entry.Name] = entry
	} else {
		r.entries[entry.Name] = entry
	}
	return r.save()
}

func (r *Registry) Get(name string) (*SoundEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	return e, ok
}

func (r *Registry) List() []*SoundEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*SoundEntry, 0, len(r.entries))
	for _, e := range r.entries {
		list = append(list, e)
	}
	return list
}

func (r *Registry) Random() *SoundEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
		return nil
	}
	keys := make([]string, 0, len(r.entries))
	for k := range r.entries {
		keys = append(keys, k)
	}
	return r.entries[keys[rand.IntN(len(keys))]]
}
