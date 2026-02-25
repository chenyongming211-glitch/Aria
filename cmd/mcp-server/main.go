package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"

	"aria/pkg/controllerstorage"
	"aria/pkg/logging"
	"aria/pkg/mcp"
)

var (
	version = "dev" // 默认开发版本，通过 ldflags 注入
)

// init 重定向所有日志输出到 stderr，避免污染 stdout 的 JSON 通信
func init() {
	// 禁用标准库的日志输出
	log.SetOutput(io.Discard)
}

func main() {
	flag.Parse()

	logger, err := logging.NewLogger(&logging.Config{
		Level:     logging.INFO,
		Component: "mcp-server",
		LogDir:    "/var/log/aria",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	logger.Info("Aria MCP Server starting...")

	// Parse PostgreSQL config from environment
	pgHost := getEnv("POSTGRES_HOST", "localhost")
	pgPort := getEnvAsInt("POSTGRES_PORT", 5432)
	pgUser := getEnv("POSTGRES_USER", "aria")
	pgPassword := getEnv("POSTGRES_PASSWORD", "")
	pgDB := getEnv("POSTGRES_DATABASE", "aria")
	pgSSLMode := getEnv("POSTGRES_SSLMODE", "disable")

	// Initialize storage
	store, err := controllerstorage.NewStorage(&controllerstorage.Config{
		Host:     pgHost,
		Port:     pgPort,
		User:     pgUser,
		Password: pgPassword,
		Database: pgDB,
		SSLMode:  pgSSLMode,
	}, "100.64.0.0", "100.64.0.0/16")
	if err != nil {
		logger.Error("Failed to connect to storage: %v", err)
		os.Exit(1)
	}
	defer store.Close()
	logger.Info("Storage connected")

	// Create tool registry with default tools
	registry := mcp.DefaultTools(logger, store)

	// Create and run MCP server
	server := mcp.NewServer("aria-mcp", version, registry)
	logger.Info("MCP Server ready, listening on stdio...")

	if err := server.Run(); err != nil {
		logger.Error("Server error: %v", err)
		os.Exit(1)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
