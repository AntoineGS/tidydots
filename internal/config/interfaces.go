package config

// Loader defines the interface for loading configuration
type Loader interface {
	Load(path string) (*Config, error)
}

// Validator defines the interface for configuration validation
type Validator interface {
	Validate() error
}

// Filterable defines types that can be filtered by context
type Filterable interface {
	MatchesFilter(ctx *FilterContext) bool
}

// EntryGetter defines interface for retrieving entries
type EntryGetter interface {
	GetConfigEntries() []Entry
	GetGitEntries() []Entry
	GetPackageEntries() []Entry
}
