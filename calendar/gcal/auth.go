package gcal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const tokenFile = "token.json"

func getConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".config", "calendar")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}
	return configDir, nil
}

func getTokenPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, tokenFile), nil
}

func getCredentialsPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "credentials.json"), nil
}

func GetToken(config *oauth2.Config) (*oauth2.Token, error) {
	tokPath, err := getTokenPath()
	if err != nil {
		return nil, err
	}

	tok, err := tokenFromFile(tokPath)
	if err != nil {
		tok, err = getTokenFromWeb(config)
		if err != nil {
			return nil, err
		}
		saveToken(tokPath, tok)
	}
	return tok, nil
}

func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser:\n%v\n\n", authURL)
	fmt.Print("Enter authorization code: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token from web: %v", err)
	}
	return tok, nil
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		fmt.Printf("Unable to cache oauth token: %v\n", err)
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func GetClient() (*http.Client, error) {
	credPath, err := getCredentialsPath()
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %v\nPlease create credentials.json in %s", err, credPath)
	}

	config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %v", err)
	}

	token, err := GetToken(config)
	if err != nil {
		return nil, err
	}

	return config.Client(context.Background(), token), nil
}

func GetCalendarService() (*calendar.Service, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	srv, err := calendar.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create calendar service: %v", err)
	}

	return srv, nil
}
