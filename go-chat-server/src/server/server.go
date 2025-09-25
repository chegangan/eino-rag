package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var (
	mu          sync.Mutex
	chatHistory = make([]*schema.Message, 0) // Initialize as an empty slice
	cm          model.ToolCallingChatModel
)

func init() {
	if err := LoadConfig(); err != nil {
		log.Printf("Failed to load config: %v, using defaults", err)
	}
	// Don't create chat model here, create it on demand
}

func clearHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		mu.Lock()
		defer mu.Unlock()
		chatHistory = make([]*schema.Message, 0) // Reset the history
		log.Println("Chat history cleared.")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("History cleared"))
		return
	}
	http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
}
func chatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid input", http.StatusBadRequest)
			return
		}

		// Get the latest system prompt
		currentPrompt := GetSystemPrompt()

		// Check if we need to create or recreate the chat model
		mu.Lock()
		if cm == nil {
			ctx := context.Background()
			newCm, err := createOpenAIChatModel(ctx)
			if err != nil {
				mu.Unlock()
				http.Error(w, fmt.Sprintf("Chat model not configured: %v. Please set a valid API key in the settings.", err), http.StatusBadRequest)
				return
			}
			cm = newCm
		}
		mu.Unlock()

		// Process the chat message using the current system prompt
		messages := buildMessages(chatHistory, req.Message, currentPrompt)
		streamResult, err := stream(r.Context(), cm, messages)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to start stream: %v", err), http.StatusInternalServerError)
			return
		}
		defer streamResult.Close()

		// Switch to streaming response
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		bw := bufio.NewWriter(w)
		var assistantMsg string
		for {
			// 在接收数据前，先检查上下文是否已被取消
			select {
			case <-r.Context().Done():
				log.Println("Request context cancelled, stopping stream.")
				return // 直接返回，允许服务器关闭
			default:
				// 上下文未取消，继续执行 Recv()
			}

			message, err := streamResult.Recv()
			if err != nil {
				if err == io.EOF {
					break // 流正常结束
				}
				// 再次检查错误是否由上下文取消引起
				if r.Context().Err() != nil {
					log.Println("Stream stopped due to context cancellation.")
					return // 直接返回
				}
				// 其他类型的错误
				log.Printf("Stream recv failed: %v", err)
				writeSSE(bw, flusher, "error", fmt.Sprintf("recv failed: %v", err))
				return
			}
			chunk := message.Content
			assistantMsg += chunk
			// SSE data lines per newline
			writeSSE(bw, flusher, "", chunk)
		}

		// Append to chat history after completion
		chatHistory = AppendHistory(chatHistory, req.Message, assistantMsg)
		return
	}

	http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
}

// writeSSE writes an SSE message, splitting data by newlines into multiple data: lines
func writeSSE(bw *bufio.Writer, flusher http.Flusher, event string, data string) {
	if event != "" {
		fmt.Fprintf(bw, "event: %s\n", event)
	}
	// Normalize CRLF -> LF
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(bw, "data: %s\n", line)
	}
	fmt.Fprint(bw, "\n")
	bw.Flush()
	flusher.Flush()
}

func updatePromptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid input", http.StatusBadRequest)
			return
		}

		if err := UpdateSystemPrompt(req.Prompt); err != nil {
			http.Error(w, "Failed to update prompt", http.StatusInternalServerError)
			return
		}

		// Invalidate the current chat model so it gets recreated with the new prompt
		mu.Lock()
		cm = nil
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		return
	}

	http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
}

func updateConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			APIKey    string `json:"api_key"`
			BaseURL   string `json:"base_url"`
			ModelName string `json:"model_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid input", http.StatusBadRequest)
			return
		}

		// Update OpenAI configuration
		if req.APIKey != "" {
			if err := UpdateAPIKey(req.APIKey); err != nil {
				http.Error(w, "Failed to update API key", http.StatusInternalServerError)
				return
			}
		}
		if req.BaseURL != "" {
			if err := UpdateBaseURL(req.BaseURL); err != nil {
				http.Error(w, "Failed to update base URL", http.StatusInternalServerError)
				return
			}
		}
		if req.ModelName != "" {
			if err := UpdateModelName(req.ModelName); err != nil {
				http.Error(w, "Failed to update model name", http.StatusInternalServerError)
				return
			}
		}

		// Recreate the chat model with new config
		ctx := context.Background()
		newCm, err := createOpenAIChatModel(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create chat model: %v", err), http.StatusInternalServerError)
			return
		}
		cm = newCm

		response := map[string]string{"status": "Configuration updated successfully"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
}

func getConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		configLock.RLock()
		defer configLock.RUnlock()

		response := map[string]string{
			"system_prompt": config.SystemPrompt,
			// "api_key" is intentionally omitted for security
			"base_url":   config.BaseURL,
			"model_name": config.ModelName,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
}
