package gcal

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	CalendarIDs []string `json:"calendar_ids"`
}

func getConfigPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.json"), nil
}

func LoadConfig() (*Config, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(configPath)
	if err != nil {
		return &Config{CalendarIDs: []string{"primary"}}, nil
	}
	defer f.Close()

	config := &Config{}
	if err := json.NewDecoder(f).Decode(config); err != nil {
		return &Config{CalendarIDs: []string{"primary"}}, nil
	}

	if len(config.CalendarIDs) == 0 {
		config.CalendarIDs = []string{"primary"}
	}

	return config, nil
}

func SaveConfig(config *Config) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(configPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(config)
}
