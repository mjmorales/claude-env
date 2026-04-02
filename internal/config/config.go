package config

// Environment represents a single Claude Code subscription environment.
type Environment struct {
	Credentials      string   `mapstructure:"credentials"`
	Shared           []string `mapstructure:"shared"`
	SettingsOverride string   `mapstructure:"settings_override"`
}

// Pool defines the shared resource pool location.
type Pool struct {
	Path string `mapstructure:"path"`
}

// Config is the top-level claude-env configuration.
type Config struct {
	Environments map[string]Environment `mapstructure:"environments"`
	Pool         Pool                   `mapstructure:"pool"`
	Global       string                 `mapstructure:"global"`
}
