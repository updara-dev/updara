package connector

// Connector mirrors the YAML connector file format.
type Connector struct {
	Name          string        `yaml:"name"`
	DisplayName   string        `yaml:"display_name"`
	Icon          string        `yaml:"icon"`
	Category      string        `yaml:"category"`
	Docs          string        `yaml:"docs"`
	Check         Check         `yaml:"check"`
	Update        Update        `yaml:"update"`
	Notifications Notifications `yaml:"notifications"`
}

type Check struct {
	Type            string            `yaml:"type"`     // shell | http
	Command         string            `yaml:"command"`  // shell only
	Endpoint        string            `yaml:"endpoint"` // http only
	Headers         map[string]string `yaml:"headers"`
	Auth            Auth              `yaml:"auth"`
	Parse           map[string]string `yaml:"parse"`
	UpdateAvailable string            `yaml:"update_available"`
	Interval        int               `yaml:"interval"` // seconds, default 3600
}

type Auth struct {
	Type  string `yaml:"type"`  // none | bearer | basic
	Token string `yaml:"token"` // supports {ENV_VAR} substitution
}

type Update struct {
	Type                 string `yaml:"type"`    // shell | hook
	Command              string `yaml:"command"` // shell only
	RequiresConfirmation bool   `yaml:"requires_confirmation"`
}

type Notifications struct {
	Changelog string `yaml:"changelog"`
}
