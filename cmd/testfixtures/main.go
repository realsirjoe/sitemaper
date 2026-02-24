package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"sitemaper/internal/testserver"
)

func main() {
	suite := testserver.NewSuite()
	defer suite.Close()

	fmt.Println("Sitemaper fixture servers running:")
	fmt.Printf("  no-sitemap : %s\n", suite.NoSitemap.URL)
	fmt.Printf("  root-only  : %s\n", suite.RootOnly.URL)
	fmt.Printf("  robots-only: %s\n", suite.RobotsOnly.URL)
	fmt.Printf("  deep-a     : %s\n", suite.DeepA.URL)
	fmt.Printf("  deep-b     : %s\n", suite.DeepB.URL)
	fmt.Printf("  malformed  : %s\n", suite.Malformed.URL)
	fmt.Println("")
	fmt.Println("Try:")
	fmt.Printf("  curl %s/sitemap.xml\n", suite.RootOnly.URL)
	fmt.Printf("  curl %s/robots.txt\n", suite.RobotsOnly.URL)
	fmt.Printf("  curl %s/sitemap.xml\n", suite.DeepA.URL)
	fmt.Printf("  curl %s/x/child.xml\n", suite.DeepB.URL)
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop.")

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	<-sigc
}
