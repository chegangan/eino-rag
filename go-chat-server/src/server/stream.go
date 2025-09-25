package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cloudwego/eino/schema"
)

func reportStream(sr *schema.StreamReader[*schema.Message]) (string, error) {
	defer sr.Close()
	var assistantMsg string

	fmt.Print("assistant: ")
	for {
		message, err := sr.Recv()
		if err == io.EOF {
			fmt.Println() // Add a newline for cleaner terminal output
			return assistantMsg, nil
		}
		if err != nil {
			return "", fmt.Errorf("recv failed: %v", err)
		}
		content := message.Content
		if assistantMsg == "" {
			content = strings.TrimLeft(content, "\n")
		}
		fmt.Print(content)
		assistantMsg += content
	}
}

func ReadUserInput() string {
	reader := bufio.NewReader(os.Stdin)
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
