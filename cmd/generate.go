package cmd

import (
	"bytes"
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
	name      string
	sensitive bool
	expr      *hclwrite.Expression
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

	providerReqExists := false

	for _, file := range files {
		body := file.Body()

		if gc.checkProviderRequirementsExists(body) {
			providerReqExists = true
		}

		for _, block := range body.Blocks() {
			blockBody := block.Body()

			if block.Type() == "output" {
				outputs = append(outputs, &outputDefinitions{
					name:      block.Labels()[0],
					sensitive: checkSensitiveAttr(blockBody),
					expr:      blockBody.GetAttribute("value").Expr(),
				})
			}
		}
	}

	if len(outputs) == 0 {
		log.Printf("No outputs defined, skipping.")
		return nil
	}

	file := gc.buildContext(outputs)

	if !providerReqExists {
		providerFile := gc.buildProviderRequirements()

		err = ioutil.WriteFile(spaceliftOverrideFile, providerFile.Bytes(), os.ModePerm)
		if err != nil {
			return err
		}
	}

	err = ioutil.WriteFile(gc.outputFile, file.Bytes(), os.ModePerm)
	if err != nil {
		return err
	}

	log.Println("Finished creating spacelift context file")

	return nil
}

func (gc *generateCmd) checkProviderRequirementsExists(body *hclwrite.Body) bool {
	exists := false

	for _, block := range body.Blocks() {
		if block.Type() == "terraform" {
			for _, tBlock := range block.Body().Blocks() {
				if tBlock.Type() == "required_providers" {
					for name := range tBlock.Body().Attributes() {
						if name == "spacelift" {
							exists = true
						}
					}
				}
			}
		}
	}

	return exists
}

func (gc *generateCmd) buildProviderRequirements() *hclwrite.File {
	file := hclwrite.NewEmptyFile()

	block := file.Body().AppendNewBlock("terraform", []string{})
	providerBlock := block.Body().AppendNewBlock("required_providers", []string{})
	providerBlock.Body().SetAttributeValue("spacelift", cty.ObjectVal(map[string]cty.Value{
		"source":  cty.StringVal("spacelift-io/spacelift"),
		"version": cty.StringVal(fmt.Sprintf("~> %s", spaceliftProviderVersion)),
	}))

	return file
}

func (gc *generateCmd) buildContext(outputs []*outputDefinitions) *hclwrite.File {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	contextBlock := body.AppendNewBlock("resource", []string{"spacelift_context", "outputs"})
	contextBlock.Body().SetAttributeValue("name", cty.StringVal(gc.contextName))
	contextBlock.Body().SetAttributeValue("description", cty.StringVal("Auto generated context by spacectx"))

	localsBlock := body.AppendNewBlock("locals", []string{})

	for _, output := range outputs {
		localsBlock.Body().SetAttributeRaw(fmt.Sprintf("out_%s", output.name), output.expr.BuildTokens(nil))
	}

	if checkIfAny(outputs, func(o *outputDefinitions) bool { return !o.sensitive }) {
		gc.appendFileBlock(body, outputs, false, contextFileName, "out_sctx_content")
	}
	if checkIfAny(outputs, func(o *outputDefinitions) bool { return o.sensitive }) {
		gc.appendFileBlock(body, outputs, true, contextSecretsFileName, "out_sctx_content_secrets")
	}

	return file
}

func (gc *generateCmd) appendFileBlock(body *hclwrite.Body, outputs []*outputDefinitions, sensitive bool, fileName string, localAttributeName string) {
	fileBlock := body.AppendNewBlock("resource", []string{"spacelift_mounted_file", localAttributeName})
	fileBlock.Body().SetAttributeTraversal("context_id", hcl.Traversal{
		hcl.TraverseRoot{
			Name: "spacelift_context",
		},
		hcl.TraverseAttr{
			Name: "outputs",
		},
		hcl.TraverseAttr{
			Name: "id",
		},
	})
	fileBlock.Body().SetAttributeValue("relative_path", cty.StringVal(fmt.Sprintf(fileName, gc.contextName)))
	fileBlock.Body().SetAttributeValue("write_only", cty.BoolVal(sensitive))
	fileBlock.Body().SetAttributeTraversal("content", hcl.Traversal{
		hcl.TraverseRoot{
			Name: "local",
		},
		hcl.TraverseAttr{
			Name: localAttributeName,
		},
	})

	localsBlock := body.FirstMatchingBlock("locals", []string{})

	unencodedName := fmt.Sprintf("%s_raw", localAttributeName)

	localsBlock.Body().SetAttributeRaw(unencodedName, localsContent(outputs, sensitive).BuildTokens(nil))
	localsBlock.Body().SetAttributeRaw(localAttributeName, localsContentEncoded(unencodedName).BuildTokens(nil))
}

func checkSensitiveAttr(body *hclwrite.Body) bool {
	attr := body.GetAttribute("sensitive")
	if attr == nil {
		return false
	}

	// Only checking if it contains a single token which has value `true`
	tokens := attr.Expr().BuildTokens(nil)
	return len(tokens) == 1 && bytes.Equal(tokens[0].Bytes, []byte(`true`))
}

func checkIfAny(outputs []*outputDefinitions, pred func(*outputDefinitions) bool) bool {
	for _, output := range outputs {
		if pred(output) {
			return true
		}
	}

	return false
}

func localsContent(outputs []*outputDefinitions, sensitive bool) hclwrite.Tokens {
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
		if output.sensitive == sensitive {
			localsContent = append(localsContent, localsOutputContent(output.name)...)
		}
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

func localsContentEncoded(unencodedName string) hclwrite.Tokens {
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
			Bytes:        []byte(unencodedName),
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
