package main

import (
	"context"
	"os"

	"sitemaper/internal/app"
)

func main() {
	code := app.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, app.Config{})
	os.Exit(code)
}
