package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// State is persisted UI/runtime config (keys + gateway defaults).
type State struct {
	Keys     Keys     `json:"keys"`
	Defaults Defaults `json:"defaults"`
}

type Keys struct {
	GeminiAPIKey     string `json:"gemini_api_key"`
	GroqAPIKey       string `json:"groq_api_key"`
	OpenRouterAPIKey string `json:"openrouter_api_key"`
}

type Defaults struct {
	Model               string   `json:"model"` // "auto" or model id
	Temperature         *float64 `json:"temperature,omitempty"`
	TopP                *float64 `json:"top_p,omitempty"`
	TopK                *int     `json:"top_k,omitempty"`
	MaxTokens           *int     `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int     `json:"max_completion_tokens,omitempty"`
	PresencePenalty     *float64 `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64 `json:"frequency_penalty,omitempty"`
	Seed                *int64   `json:"seed,omitempty"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	cur  State
}

func Open(dir string) (*Store, error) {
	if dir == "" {
		dir = ".runtime"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "state.json")
	s := &Store{path: path, cur: State{Defaults: Defaults{Model: "auto"}}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			_ = s.Save(s.cur)
			return s, nil
		}
		return nil, err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &s.cur); err != nil {
			return nil, err
		}
	}
	if s.cur.Defaults.Model == "" {
		s.cur.Defaults.Model = "auto"
	}
	if fixed, changed := sanitizeDefaults(s.cur.Defaults); changed {
		s.cur.Defaults = fixed
		_ = s.persistLocked()
	}
	return s, nil
}

// sanitizeDefaults drops/repairs invalid sampling defaults from older UI saves.
func sanitizeDefaults(d Defaults) (Defaults, bool) {
	before, _ := json.Marshal(d)
	if d.TopP != nil && (*d.TopP < 0 || *d.TopP > 1) {
		d.TopP = nil
	}
	if d.Temperature != nil && (*d.Temperature < 0 || *d.Temperature > 2) {
		d.Temperature = nil
	}
	if d.TopK != nil && (*d.TopK < 1 || *d.TopK > 200) {
		d.TopK = nil
	}
	if d.MaxTokens != nil && (*d.MaxTokens < 8 || *d.MaxTokens > 128_000) {
		d.MaxTokens = nil
	}
	if d.PresencePenalty != nil && (*d.PresencePenalty < -2 || *d.PresencePenalty > 2) {
		d.PresencePenalty = nil
	}
	if d.FrequencyPenalty != nil && (*d.FrequencyPenalty < -2 || *d.FrequencyPenalty > 2) {
		d.FrequencyPenalty = nil
	}
	after, _ := json.Marshal(d)
	return d, string(before) != string(after)
}

func (s *Store) persistLocked() error {
	data, err := json.MarshalIndent(s.cur, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) Get() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

func (s *Store) Save(st State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.Defaults.Model == "" {
		st.Defaults.Model = "auto"
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.cur = st
	return nil
}

func (s *Store) SetupComplete() bool {
	st := s.Get()
	return st.Keys.GeminiAPIKey != "" || st.Keys.GroqAPIKey != "" || st.Keys.OpenRouterAPIKey != ""
}

// MergeEnvKeys fills empty runtime keys from environment (one-time bootstrap aid).
func (s *Store) MergeEnvKeys(gemini, groq, openrouter string) (bool, error) {
	st := s.Get()
	changed := false
	if st.Keys.GeminiAPIKey == "" && gemini != "" {
		st.Keys.GeminiAPIKey = gemini
		changed = true
	}
	if st.Keys.GroqAPIKey == "" && groq != "" {
		st.Keys.GroqAPIKey = groq
		changed = true
	}
	if st.Keys.OpenRouterAPIKey == "" && openrouter != "" {
		st.Keys.OpenRouterAPIKey = openrouter
		changed = true
	}
	if !changed {
		return false, nil
	}
	return true, s.Save(st)
}

func MaskKey(k string) string {
	k = trim(k)
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "••••"
	}
	return k[:4] + "••••" + k[len(k)-4:]
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}
