package config

const (
	configKeyWidth = "width"
	configKeyToken = "token"
)

// non-secret -> get_width + set_width ; secret -> set_token only
var _ = genConfigOption(configKeyWidth, "width", 0)
var _ = genSecretConfigOption(configKeyToken, "token", "")
