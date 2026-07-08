// earsaudiobridge-server exposes a virtual sink on the host, streams its audio to
// connected clients and plays received client mic audio into a virtual source.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/earentir/audiobridge/internal/audio/pick"
	"github.com/earentir/audiobridge/internal/cli"
	"github.com/earentir/audiobridge/internal/config"
	"github.com/earentir/audiobridge/internal/server"
)

var appVersion = "0.1.114"

func main() {
	var configFlag string

	root := &cobra.Command{
		Use:     "earsaudiobridge-server",
		Short:   "Ears Audio Bridge server",
		Version: appVersion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, err := cli.ConfigPath(configFlag, "server.yaml")
			if err != nil {
				return err
			}
			return run(cfgPath)
		},
	}
	root.Flags().StringVar(&configFlag, "config", "server.yaml", "config file (relative to the current directory)")

	cli.Execute(root)
}

func run(cfgPath string) error {
	cfg, err := config.LoadServer(cfgPath)
	if err != nil {
		return err
	}

	sys, err := pick.ServerSystem(cfg.Backend)
	if err != nil {
		return err
	}
	log.Printf("audio backend: %s", sys.Name())

	srv := server.New(cfg, sys, appVersion)
	if err := srv.Setup(); err != nil {
		return err
	}
	log.Printf("audio setup complete")

	go func() {
		if err := srv.ServeWeb(); err != nil {
			log.Fatalf("web ui: %v", err)
		}
	}()

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("stream listener: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Print("shutting down")
	srv.Shutdown()
	return nil
}
