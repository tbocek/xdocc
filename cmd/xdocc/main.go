// Command xdocc compiles a source tree into a static site.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/tbocek/xdocc/internal/xdocc"
)

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	var (
		source    = flag.String("s", "", "`directory` to read the site from")
		output    = flag.String("o", "", "`directory` to write the site to")
		cachePath = flag.String("c", "", "cache `file`; without it the cache lives in memory")
		watch     = flag.Bool("w", false, "keep running and recompile when the source changes")
		clear     = flag.Bool("x", false, "clear the cache before compiling")
		showVer   = flag.Bool("v", false, "print the version and exit")
	)
	// long forms, for scripts that prefer to be readable
	flag.StringVar(source, "source", "", "")
	flag.StringVar(output, "output", "", "")
	flag.StringVar(cachePath, "cache", "", "")
	flag.BoolVar(watch, "watch", false, "")
	flag.BoolVar(clear, "clear", false, "")
	flag.BoolVar(showVer, "version", false, "")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: xdocc -s <source> -o <output> [-c <cache>] [-w] [-x]\n\n")
		fmt.Fprintf(os.Stderr, "  -s, -source directory   read the site from here\n")
		fmt.Fprintf(os.Stderr, "  -o, -output directory   write the site to here\n")
		fmt.Fprintf(os.Stderr, "  -c, -cache file         cache file; without it the cache lives in memory\n")
		fmt.Fprintf(os.Stderr, "  -w, -watch              keep running and recompile when the source changes\n")
		fmt.Fprintf(os.Stderr, "  -x, -clear              clear the cache before compiling\n")
		fmt.Fprintf(os.Stderr, "  -v, -version            print the version and exit\n")
	}
	flag.Parse()

	if *showVer {
		fmt.Printf("xdocc %s\n", version)
		return
	}
	if *source == "" || *output == "" {
		flag.Usage()
		os.Exit(2)
	}
	log.SetFlags(0)

	site, err := xdocc.NewSite(*source, *output)
	if err != nil {
		log.Fatalf("xdocc: %v", err)
	}
	cache := xdocc.OpenCache(*cachePath)
	if *clear {
		cache.Clear()
	}
	site.SetCache(cache)

	if !*watch {
		result, err := site.Compile()
		if err != nil {
			log.Fatalf("xdocc: %v", err)
		}
		log.Printf("xdocc: %s", result)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("xdocc: watching %s", site.Source)
	if err := site.Watch(ctx); err != nil {
		log.Fatalf("xdocc: %v", err)
	}
}
