package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nunoOliveiraqwe/torii"
	"go.uber.org/zap"
)

const banner = `
  _              _ _ 
 | |_ ___  _ __ (_|_)
 | __/ _ \| '_ \| | |
 | || (_) | |   | | |
  \__\___/|_|   |_|_|
`

func printBanner() {
	fmt.Print(banner)
	fmt.Printf("  Version:    %s\n", torii.Version)
	fmt.Printf("  Build:      %s\n", torii.Build)
	fmt.Printf("  Build Time: %s\n", torii.BuildTime)
	fmt.Println()
}

func main() {
	printBanner()

	app := torii.NewApplication()
	app.ParseFlags()

	if err := app.LoadConfiguration(); err != nil {
		fmt.Print(fmt.Errorf("failed to load configuration: %v", err))
	}
	app.InitLogger()

	if err := app.Validate(); err != nil {
		zap.S().Fatalf("Invalid application state: %v", err)
	}

	if err := app.Start(); err != nil {
		zap.S().Fatalf("Failed to start application: %v", err)
	}

	zap.S().Infof("Application started successfully. Listening for shutdown signals...")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	zap.S().Infof("Received signal: %v. Shutting down...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.Shutdown(ctx); err != nil {
		zap.S().Fatalf("Shutdown failed: %v", err)
	}
}
