package goansible

type Config struct {
	ShowCommandOutput bool
	Debug             bool
	Passphrase        string
}

var DefaultConfig = &Config{false, false, ""}
