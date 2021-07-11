package cmd

import (
	"github.com/2ttech/spacectx/internal/templates"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	rootLong = templates.LongDesc(`Spacectx is a simple tool used in Spacelift stacks to auto-generate context
			based on the outputs from module. If the context is attached to a stack it can also replace the
			variables in tfvars file with values from context.`)
)

var (
	debug bool
)

const (
	contextFileName        = "ctx-%v.json"
	contextSecretsFileName = "ctx-%v-secrets.json"
)

// NewRootCmd returns the root command for utility
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "spacectx",
		Short: "Spacectx generates spacelift context and manage manage variable files.",
		Long:  rootLong,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if debug {
				log.SetLevel(log.DebugLevel)
			}
		},
	}

	p := rootCmd.PersistentFlags()
	p.BoolVar(&debug, "debug", false, "enable verbose debug logs")

	rootCmd.AddCommand(newGenerateCmd())
	rootCmd.AddCommand(newProcessCmd())

	return rootCmd
}
