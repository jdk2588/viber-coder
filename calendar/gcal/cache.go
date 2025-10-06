package gcal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type EventCache struct {
	Year      int                            `json:"year"`
	Events    map[time.Month]map[int][]Event `json:"events"`
	Timestamp time.Time                      `json:"timestamp"`
}

func getCachePath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "events_cache.json"), nil
}

func SaveEventsCache(year int, events map[time.Month]map[int][]Event) error {
	cachePath, err := getCachePath()
	if err != nil {
		return err
	}

	cache := EventCache{
		Year:      year,
		Events:    events,
		Timestamp: time.Now(),
	}

	f, err := os.OpenFile(cachePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(cache)
}

func LoadEventsCache(year int) (map[time.Month]map[int][]Event, bool) {
	cachePath, err := getCachePath()
	if err != nil {
		return nil, false
	}

	f, err := os.Open(cachePath)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	var cache EventCache
	if err := json.NewDecoder(f).Decode(&cache); err != nil {
		return nil, false
	}

	if cache.Year != year {
		return nil, false
	}

	cacheAge := time.Since(cache.Timestamp)
	if cacheAge > 24*time.Hour {
		return cache.Events, false
	}

	return cache.Events, true
}

func ClearEventsCache() error {
	cachePath, err := getCachePath()
	if err != nil {
		return err
	}
	return os.Remove(cachePath)
}
