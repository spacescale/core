package domain

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrInvalidAppName = errors.New("invalid app name")
	ErrInvalidImage   = errors.New("invalid imag ref")
	ErrInvalidPort    = errors.New("invalid port")

	// lowercase letters + digits, seperated by single hyphens
	appNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

func ValidateAppName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || !appNameRe.MatchString(name) {
		return ErrInvalidAppName
	}
	return nil
}

func ValidateImageRef(image string) error {
	if strings.TrimSpace(image) == "" {
		return ErrInvalidImage
	}
	//v0: we keep permissive for now parsing wil come later
	return nil
}

func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return ErrInvalidPort
	}
	return nil
}
