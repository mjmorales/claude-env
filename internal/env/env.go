package env

// Manager handles environment switching and symlink reconciliation.
type Manager struct {
	EnvsDir  string
	ClaudeDir string
}

// New creates an environment manager.
func New(envsDir, claudeDir string) *Manager {
	return &Manager{
		EnvsDir:   envsDir,
		ClaudeDir: claudeDir,
	}
}
