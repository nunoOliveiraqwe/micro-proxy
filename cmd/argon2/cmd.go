package main

import (
	"fmt"
	"os"

	"github.com/nunoOliveiraqwe/torii/internal/auth"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: argon2 <password>")
		os.Exit(1)
	}

	password := os.Args[1]
	encoder := auth.NewDefaultEncoder()

	hashed, err := encoder.Encrypt(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encrypting password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(hashed)
}
