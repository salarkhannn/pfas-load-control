package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const defaultMireyeBaseURL = "https://api.mireye.com"

type Config struct {
	DatabaseURL string
	MireyeToken string
	MireyeURL   string
	HTTPAddress string
	WebOrigin   string
	LogLevel    slog.Level
}

func Load() (Config, error) {
	databaseURL, err := required("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}

	mireyeToken := strings.TrimSpace(os.Getenv("MIREYE_API_TOKEN"))
	if mireyeToken == "" {
		mireyeToken = strings.TrimSpace(os.Getenv("MIREYE_TOKEN"))
	}
	if mireyeToken == "" {
		return Config{}, errors.New("missing required environment variable MIREYE_API_TOKEN (or MIREYE_TOKEN)")
	}

	mireyeURL := valueOrDefault("MIREYE_BASE_URL", defaultMireyeBaseURL)
	if err := validateHTTPURL("MIREYE_BASE_URL", mireyeURL); err != nil {
		return Config{}, err
	}

	webOrigin := valueOrDefault("WEB_ORIGIN", "http://localhost:5173")
	if err := validateHTTPURL("WEB_ORIGIN", webOrigin); err != nil {
		return Config{}, err
	}

	logLevel := new(slog.LevelVar)
	if err := logLevel.UnmarshalText([]byte(valueOrDefault("LOG_LEVEL", "INFO"))); err != nil {
		return Config{}, fmt.Errorf("invalid LOG_LEVEL: %w", err)
	}

	httpAddress, err := loadHTTPAddress()
	if err != nil {
		return Config{}, err
	}

	return Config{
		DatabaseURL: databaseURL,
		MireyeToken: mireyeToken,
		MireyeURL:   strings.TrimRight(mireyeURL, "/"),
		HTTPAddress: httpAddress,
		WebOrigin:   strings.TrimRight(webOrigin, "/"),
		LogLevel:    logLevel.Level(),
	}, nil
}

func loadHTTPAddress() (string, error) {
	if address := strings.TrimSpace(os.Getenv("HTTP_ADDRESS")); address != "" {
		return address, nil
	}
	if portValue := strings.TrimSpace(os.Getenv("PORT")); portValue != "" {
		port, err := strconv.Atoi(portValue)
		if err != nil || port < 1 || port > 65535 {
			return "", errors.New("invalid PORT: must be an integer from 1 through 65535")
		}
		return ":" + portValue, nil
	}
	return ":8080", nil
}

func required(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("missing required environment variable %s", name)
	}
	return value, nil
}

func valueOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func validateHTTPURL(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid %s: must be an absolute HTTP(S) URL", name)
	}
	return nil
}
