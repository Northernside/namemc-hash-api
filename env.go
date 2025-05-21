package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"
)

var (
	env = make(map[string]string)
)

func loadEnvironment() {
	file, err := os.Open(".env")
	if err != nil {
		log.Fatal("Failed to open .env file:", err)
	}
	defer file.Close()

	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, file)
	if err != nil {
		log.Fatal("Failed to read .env file:", err)
	}

	lines := strings.SplitSeq(buffer.String(), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			env[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
}

func getEnv(key string) string {
	if value, exists := env[key]; exists {
		return value
	}

	log.Printf("Warning: Environment variable %s not found\n", key)
	return ""
}
