package templates

import "embed"

//go:embed identity.md user.md
var FS embed.FS

// Identity returns the default identity template
func Identity() (string, error) {
	data, err := FS.ReadFile("identity.md")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// User returns the default user template
func User() (string, error) {
	data, err := FS.ReadFile("user.md")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
