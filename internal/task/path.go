package task

import (
	"os"
	"path/filepath"
	"sync"
)

func DefaultDataDir() (string, error) {
	if p := os.Getenv("TODO_GO_DATA"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".todo-go"), nil
}

func UserStorePath(username string) (string, error) {
	base, err := DefaultDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "users", username, "tasks.json"), nil
}

func OpenForUser(username string) (*Store, error) {
	path, err := UserStorePath(username)
	if err != nil {
		return nil, err
	}
	return OpenStore(path)
}

func MigrateLegacy(targetUser string) (bool, error) {
	base, err := DefaultDataDir()
	if err != nil {
		return false, err
	}
	legacy := filepath.Join(base, "tasks.json")
	if _, err := os.Stat(legacy); err != nil {
		return false, nil
	}
	newPath, err := UserStorePath(targetUser)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(newPath); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(legacy, newPath); err != nil {
		return false, err
	}
	return true, nil
}

type Manager struct {
	mu     sync.Mutex
	stores map[string]*Store
}

func NewManager() *Manager {
	return &Manager{stores: make(map[string]*Store)}
}

func (m *Manager) ForUser(username string) (*Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.stores[username]; ok {
		return s, nil
	}
	s, err := OpenForUser(username)
	if err != nil {
		return nil, err
	}
	m.stores[username] = s
	return s, nil
}
