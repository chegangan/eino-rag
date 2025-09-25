package main

import (
	"encoding/json"
	"os"
	"sync"
)

type Config struct {
	SystemPrompt string `json:"system_prompt"`
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	ModelName    string `json:"model_name"`
}

var (
	config     Config
	configLock sync.RWMutex
	configFile = "./config.json"
)

// LoadConfig loads the configuration from a JSON file
func LoadConfig() error {
	configLock.Lock()
	defer configLock.Unlock()

	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default config if file doesn't exist
			config = Config{
				SystemPrompt: "你是一个中英互译专家。",
				APIKey:       "your_api_key_here",
				BaseURL:      "https://api.siliconflow.cn/v1",
				ModelName:    "Qwen/Qwen3-8B",
			}
			return SaveConfig()
		}
		return err
	}
	return json.Unmarshal(data, &config)
}

// SaveConfig saves the configuration to a JSON file
func SaveConfig() error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0644)
}

// GetSystemPrompt returns the current system prompt
func GetSystemPrompt() string {
	configLock.RLock()
	defer configLock.RUnlock()
	return config.SystemPrompt
}

// UpdateSystemPrompt updates the system prompt and saves it to the config file
func UpdateSystemPrompt(newPrompt string) error {
	configLock.Lock()
	defer configLock.Unlock()
	config.SystemPrompt = newPrompt
	return SaveConfig()
}

// GetAPIKey returns the current API key
func GetAPIKey() string {
	configLock.RLock()
	defer configLock.RUnlock()
	return config.APIKey
}

// UpdateAPIKey updates the API key and saves it to the config file
func UpdateAPIKey(newAPIKey string) error {
	configLock.Lock()
	defer configLock.Unlock()
	config.APIKey = newAPIKey
	return SaveConfig()
}

// GetBaseURL returns the current base URL
func GetBaseURL() string {
	configLock.RLock()
	defer configLock.RUnlock()
	return config.BaseURL
}

// UpdateBaseURL updates the base URL and saves it to the config file
func UpdateBaseURL(newBaseURL string) error {
	configLock.Lock()
	defer configLock.Unlock()
	config.BaseURL = newBaseURL
	return SaveConfig()
}

// GetModelName returns the current model name
func GetModelName() string {
	configLock.RLock()
	defer configLock.RUnlock()
	return config.ModelName
}

// UpdateModelName updates the model name and saves it to the config file
func UpdateModelName(newModelName string) error {
	configLock.Lock()
	defer configLock.Unlock()
	config.ModelName = newModelName
	return SaveConfig()
}
