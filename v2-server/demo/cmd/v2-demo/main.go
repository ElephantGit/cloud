//go:build linux

// Command v2-demo prepares one recording environment for scripts/demo.sh:
//
//	v2-demo env <newcomer|lead|ops|all>    print the shell exports for the environment
//	v2-demo reset <environment>            empty its data directory and recreate its database
//	v2-demo seed <environment>             write its scenes into the running v2-server's database
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wanglongan587/cloud/v2-server/demo"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "v2-demo:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: v2-demo env|reset|seed newcomer|lead|ops|all")
	}
	config, err := demo.Resolve(args[1], os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	switch args[0] {
	case "env":
		fmt.Print(config.Shell())
		return nil
	case "reset":
		return demo.Reset(ctx, config)
	case "seed":
		seeder, err := demo.OpenSeeder(ctx, config)
		if err != nil {
			return err
		}
		defer seeder.Close()
		return seeder.Seed(ctx, config.Scenes)
	}
	return fmt.Errorf("unknown command %q", args[0])
}
