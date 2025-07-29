package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/cloudwego/eino/schema"
)

func reportStream(sr *schema.StreamReader[*schema.Message]) string {
	defer sr.Close()
	var assistantMsg string

	fmt.Print("assistant: ")
	// 逐条接收消息并打印
	for {
		message, err := sr.Recv()
		if err == io.EOF {
			return assistantMsg
		}
		if err != nil {
			log.Fatalf("recv failed: %v", err)
		}
		content := message.Content
		// 去除开头的2个换行
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
		if line == "" { // 遇到空行结束
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, `\n`)
}
