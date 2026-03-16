package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	microProxy "github.com/nunoOliveiraqwe/micro-proxy"
	"go.uber.org/zap"
)

const banner = `
           _                ____                       
 _ __ ___ (_) ___ _ __ ___ |  _ \ _ __ _____  ___   _ 
| '_ ` + "`" + ` _ \| |/ __| '__/ _ \| |_) | '__/ _ \ \/ / | | |
| | | | | | | (__| | | (_) |  __/| | | (_) >  <| |_| |
|_| |_| |_|_|\___|_|  \___/|_|   |_|  \___/_/\_\\__, |
                                                  |___/ `

func printBanner() {
	fmt.Println(banner)
	fmt.Printf("  Version:    %s\n", microProxy.Version)
	fmt.Printf("  Build:      %s\n", microProxy.Build)
	fmt.Printf("  Build Time: %s\n", microProxy.BuildTime)
	fmt.Println()
}

func main() {
	printBanner()

	app := microProxy.NewApplication()
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
