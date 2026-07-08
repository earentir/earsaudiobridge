// earsaudiobridge-client connects to an EarsAudioBridge server, plays the received
// stream like any media application and optionally sends the local mic back.
package main

import (
	"github.com/spf13/cobra"

	"github.com/earentir/audiobridge/internal/cli"
	"github.com/earentir/audiobridge/internal/clientapp"
)

var appVersion = "0.1.148"

func main() {
	var configFlag string
	var trayFlag bool

	root := &cobra.Command{
		Use:     "earsaudiobridge-client",
		Short:   "Ears Audio Bridge client",
		Version: appVersion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, err := cli.ConfigPath(configFlag, "client.yaml")
			if err != nil {
				return err
			}
			return clientapp.Run(cfgPath, appVersion, trayFlag)
		},
	}
	root.Flags().StringVar(&configFlag, "config", "client.yaml", "config file (relative to the current directory)")
	root.Flags().BoolVar(&trayFlag, "tray", false, "show a system tray icon (open web UI, reconnect, quit)")

	cli.Execute(root)
}
