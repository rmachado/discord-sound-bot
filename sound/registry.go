package sound

import (
	"encoding/json"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	dirPath string
}

func NewRegistry(path string) (*Registry, error) {
	r := &Registry{
		entries: make(map[string]*SoundEntry),
		path:    path,
		dirPath: filepath.Dir(path),
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
	r.entries[entry.Name] = entry
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

func (r *Registry) AutoRegister() int {
	r.mu.RLock()
	existingEntries := make(map[string]bool, len(r.entries))
	for k := range r.entries {
		existingEntries[k] = true
	}
	r.mu.RUnlock()

	type pending struct {
		path string
		name string
	}
	var dcaPending []pending

	for _, path := range findFiles(r.dirPath, ".dca") {
		name := nameFromFile(path, ".dca")
		if name == "" || strings.HasPrefix(name, ".") || existingEntries[name] {
			continue
		}
		dcaPending = append(dcaPending, pending{path, name})
	}

	sourceExts := []string{".mp3", ".wav", ".ogg", ".m4a", ".webm", ".flac", ".opus", ".aac", ".wma"}
	var convertPending []pending

	for _, ext := range sourceExts {
		for _, path := range findFiles(r.dirPath, ext) {
			name := nameFromFile(path, ext)
			if name == "" || strings.HasPrefix(name, ".") || existingEntries[name] {
				continue
			}
			dcaOnDisk := filepath.Join(r.dirPath, name+".dca")
			if _, err := os.Stat(dcaOnDisk); err == nil {
				continue
			}
			convertPending = append(convertPending, pending{path, name})
		}
	}

	for _, p := range convertPending {
		dcaPath := filepath.Join(r.dirPath, p.name+".dca")
		log.Printf("[REGISTRY] converting %s -> %s.dca", filepath.Base(p.path), p.name)
		if err := EncodeToDCA(p.path, dcaPath, nil); err != nil {
			log.Printf("[REGISTRY] failed to convert %s: %v", filepath.Base(p.path), err)
			continue
		}
		os.Remove(p.path)
		log.Printf("[REGISTRY] converted and removed %s", filepath.Base(p.path))
		dcaPending = append(dcaPending, pending{dcaPath, p.name})
	}

	if len(dcaPending) == 0 {
		return 0
	}

	r.mu.Lock()
	added := 0
	for _, p := range dcaPending {
		if _, ok := r.entries[p.name]; ok {
			continue
		}
		r.entries[p.name] = &SoundEntry{
			Name:    p.name,
			File:    p.name + ".dca",
			AddedAt: time.Now().Format(time.RFC3339),
		}
		added++
	}
	if added > 0 {
		if err := r.save(); err != nil {
			log.Printf("[REGISTRY] failed to save after auto-register: %v", err)
		}
	}
	r.mu.Unlock()

	return added
}

func findFiles(dir, ext string) []string {
	pattern := filepath.Join(dir, "*"+ext)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return matches
}

func nameFromFile(path, ext string) string {
	base := filepath.Base(path)
	return strings.ToLower(strings.TrimSuffix(base, ext))
}
