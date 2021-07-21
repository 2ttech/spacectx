package cmd

import (
	"github.com/2ttech/spacectx/internal/templates"
	"github.com/spf13/cobra"
)

type processCmd struct {
	outputFile    string
	contextFolder string
}

var (
	processLong = templates.LongDesc(`Generate spacelift context resources based on the output resources
				in tf files. By default it searches all tf files in current folder.`)

	processExample = templates.Examples(`
		# Generate in current folder
		spacectx generate

		# Generate only from output.tf file
		spacectx generate output.tf

		# Generate based on wildcard
		spacectx generate *.tf
	`)
)

func newProcessCmd() *cobra.Command {
	pc := &processCmd{}

	processCmd := &cobra.Command{
		Use:                   "process",
		Short:                 "Process input file and replace variables from context",
		Long:                  processLong,
		Example:               processExample,
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		SilenceErrors:         true,
		Args:                  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := pc.init(); err != nil {
				return err
			}

			return pc.run(args)
		},
	}

	f := processCmd.Flags()
	f.StringVarP(&pc.outputFile, "output", "o", "", "file to write processed result to. if not set it writes to stdout")
	f.StringVarP(&pc.contextFolder, "source-folder", "s", "", "source folder to read context files from")

	return processCmd
}

func (pc *processCmd) init() error {
	return nil
}

func (pc *processCmd) run(args []string) error {
	return nil
}
