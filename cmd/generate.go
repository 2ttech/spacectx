package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/zclconf/go-cty/cty"

	"github.com/2ttech/spacectx/internal/helpers"
	"github.com/2ttech/spacectx/internal/templates"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type generateCmd struct {
	files       string
	contextName string
	outputFile  string
}

type outputDefinitions struct {
	name string
	expr *hclwrite.Expression
}

var (
	contextNameIsNotSet = errors.Errorf("context name is not set")

	generateLong = templates.LongDesc(`Generate spacelift context resources based on the output resources
				in tf files. By default it searches all tf files in current folder.`)

	generateExample = templates.Examples(`
		# Generate in current folder
		spacectx generate

		# Generate only from output.tf file
		spacectx generate output.tf

		# Generate based on wildcard
		spacectx generate *.tf
	`)
)

func newGenerateCmd() *cobra.Command {
	gc := &generateCmd{}

	generateCmd := &cobra.Command{
		Use:                   "generate [FILE/DIR]",
		Short:                 "Generate spacelift resources",
		Long:                  generateLong,
		Example:               generateExample,
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		SilenceErrors:         true,
		Args:                  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gc.init(args); err != nil {
				return err
			}

			return gc.run(args)
		},
	}

	f := generateCmd.Flags()
	f.StringVarP(&gc.contextName, "name", "n", "", "name of context to create, defaults to same as stack name")
	f.StringVarP(&gc.outputFile, "output", "o", "spacelift_context.tf", "name of output file to create, defaults to spacelift_context.tf")

	return generateCmd
}

func (gc *generateCmd) init(args []string) error {
	if len(args) == 0 {
		gc.files = "."
	} else {
		gc.files = args[0]
	}

	if gc.contextName == "" {
		gc.contextName = os.Getenv("TF_VAR_spacelift_stack_id")

		if gc.contextName == "" {
			return contextNameIsNotSet
		}

		log.Debugf("Using context name %s", gc.contextName)
	}

	return nil
}

func (gc *generateCmd) run(args []string) error {

	outputs := []*outputDefinitions{}

	files, err := helpers.ReadFiles(gc.files)

	if err != nil {
		return err
	}

	for _, file := range files {
		for _, block := range file.Body().Blocks() {
			if block.Type() == "output" {
				outputs = append(outputs, &outputDefinitions{
					name: block.Labels()[0],
					expr: block.Body().GetAttribute("value").Expr(),
				})
			}
		}
	}

	if len(outputs) == 0 {
		log.Printf("No outputs defined, skipping.")
		return nil
	}

	data := gc.buildContext(outputs)

	err = ioutil.WriteFile(gc.outputFile, data, os.ModePerm)
	if err != nil {
		return err
	}

	log.Println("Finished creating spacelift context file")

	return nil
}

func (gc *generateCmd) buildContext(outputs []*outputDefinitions) []byte {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	contextBlock := body.AppendNewBlock("resource", []string{"spacelift_context", "main"})
	contextBlock.Body().SetAttributeValue("name", cty.StringVal(gc.contextName))
	contextBlock.Body().SetAttributeValue("description", cty.StringVal("Auto generated context by spacectx"))

	fileBlock := body.AppendNewBlock("resource", []string{"spacelift_mounted_file", "main"})
	fileBlock.Body().SetAttributeTraversal("context_id", hcl.Traversal{
		hcl.TraverseRoot{
			Name: "spacelift_context",
		},
		hcl.TraverseAttr{
			Name: "main",
		},
		hcl.TraverseAttr{
			Name: "id",
		},
	})
	fileBlock.Body().SetAttributeValue("relative_path", cty.StringVal(fmt.Sprintf("ctx-%v.json", gc.contextName)))
	fileBlock.Body().SetAttributeTraversal("content", hcl.Traversal{
		hcl.TraverseRoot{
			Name: "local",
		},
		hcl.TraverseAttr{
			Name: "out_sctx_content_encoded",
		},
	})

	localsBlock := body.AppendNewBlock("locals", []string{})

	for _, output := range outputs {
		localsBlock.Body().SetAttributeRaw(fmt.Sprintf("out_%s", output.name), output.expr.BuildTokens(nil))
	}

	localsBlock.Body().SetAttributeRaw("out_sctx_content", localsContent(outputs).BuildTokens(nil))
	localsBlock.Body().SetAttributeRaw("out_sctx_content_encoded", localsContentEncoded().BuildTokens(nil))

	return file.Bytes()
}

func localsContent(outputs []*outputDefinitions) hclwrite.Tokens {
	localsContent := hclwrite.Tokens{
		{
			Type:         hclsyntax.TokenOBrace,
			Bytes:        []byte(`{`),
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenNewline,
			Bytes:        []byte{'\n'},
			SpacesBefore: 0,
		},
	}

	for _, output := range outputs {
		localsContent = append(localsContent, localsOutputContent(output.name)...)
	}

	localsContent = append(localsContent, &hclwrite.Token{
		Type:         hclsyntax.TokenCBrace,
		Bytes:        []byte(`}`),
		SpacesBefore: 0,
	})

	return localsContent
}

func localsOutputContent(name string) hclwrite.Tokens {
	return hclwrite.Tokens{
		{
			Type:         hclsyntax.TokenIdent,
			Bytes:        []byte(name),
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenEqualOp,
			Bytes:        []byte(`=`),
			SpacesBefore: 1,
		},
		{
			Type:         hclsyntax.TokenIdent,
			Bytes:        []byte(`local`),
			SpacesBefore: 1,
		},
		{
			Type:         hclsyntax.TokenDot,
			Bytes:        []byte(`.`),
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenIdent,
			Bytes:        []byte(fmt.Sprintf("out_%s", name)),
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenComma,
			Bytes:        []byte(`,`),
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenNewline,
			Bytes:        []byte{'\n'},
			SpacesBefore: 0,
		},
	}
}

func localsContentEncoded() hclwrite.Tokens {
	return hclwrite.Tokens{
		{
			Type:         hclsyntax.TokenIdent,
			Bytes:        []byte(`base64encode`),
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenOParen,
			Bytes:        []byte{'('},
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenIdent,
			Bytes:        []byte(`jsonencode`),
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenOParen,
			Bytes:        []byte{'('},
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenIdent,
			Bytes:        []byte(`local`),
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenDot,
			Bytes:        []byte(`.`),
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenIdent,
			Bytes:        []byte(`out_sctx_content`),
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenCParen,
			Bytes:        []byte{')'},
			SpacesBefore: 0,
		},
		{
			Type:         hclsyntax.TokenCParen,
			Bytes:        []byte{')'},
			SpacesBefore: 0,
		},
	}
}
