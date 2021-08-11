package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclwrite"
	log "github.com/sirupsen/logrus"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/json"

	"github.com/2ttech/spacectx/internal/templates"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type processCmd struct {
	outputFile    string
	contextFolder string
	ignoreError   bool
}

var (
	processLong = templates.LongDesc(`Process tfvars file and replaces references to values from context
		with values read from context files mounted on disk. If no context files are found it will default
		to exit with error code.`)

	processExample = templates.Examples(`
		# Process test.tfvars file
		spacectx process test.tfvars

		# Process test.tfvars file and output to processed.auto.tfvars
		spacectx process test.tfvars -o processed.auto.tfvars

		# Process test.tfvars file and ignore any errors
		spacectx process test.tfvars --ignore-errors
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
		Args:                  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := pc.init(args); err != nil {
				return err
			}

			return pc.run(args)
		},
	}

	f := processCmd.Flags()
	f.StringVarP(&pc.outputFile, "output", "o", "", "file to write processed result to. if not set it writes to stdout")
	f.StringVarP(&pc.contextFolder, "source-folder", "s", ".", "source folder to read context files from")
	f.BoolVar(&pc.ignoreError, "ignore-errors", false, "ignore any errors for variables not found")

	return processCmd
}

func (pc *processCmd) init(args []string) error {

	fn := filepath.Clean(args[0])

	_, err := os.Lstat(fn)
	if err != nil {
		return errors.Wrapf(err, "Failed to stat %v", fn)
	}

	if !strings.HasSuffix(fn, ".tfvars") {
		return errors.Errorf("Can only process tfvars files")
	}

	log.Debugf("Output file: %s", pc.outputFile)

	return nil
}

func (pc *processCmd) run(args []string) error {

	fn := args[0]

	src, err := ioutil.ReadFile(fn)
	if err != nil {
		return errors.Wrapf(err, "Failed to read file %v", fn)
	}

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(src, fn)
	if err := checkDiags(diags); err != nil {
		return err
	}

	evalContext, err := pc.generateEvalContext(file)
	if err != nil {
		return err
	}

	bytes, err := pc.processFile(file, evalContext)
	if err != nil {
		return err
	}

	if pc.outputFile == "" {
		fmt.Println(string(bytes))
	} else {
		if err := ioutil.WriteFile(pc.outputFile, bytes, os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}

func (pc *processCmd) generateEvalContext(file *hcl.File) (*hcl.EvalContext, error) {
	attrs, diags := file.Body.JustAttributes()
	if err := checkDiags(diags); err != nil {
		return nil, err
	}

	names, err := findContextsInUse(attrs)
	if err != nil {
		return nil, err
	}

	variables := map[string]cty.Value{}

	for _, name := range names {
		values := readContextFiles(pc.contextFolder, name)
		variables[name] = values
	}

	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"context": cty.ObjectVal(variables),
		},
	}, nil
}

func (pc *processCmd) processFile(file *hcl.File, context *hcl.EvalContext) ([]byte, error) {
	attrs, diags := file.Body.JustAttributes()
	if err := checkDiags(diags); err != nil {
		return nil, err
	}

	result := hclwrite.NewEmptyFile()
	for _, attr := range attrs {
		value, diags := attr.Expr.Value(context)
		if err := checkDiags(diags); err != nil {
			return nil, err
		}

		result.Body().SetAttributeValue(attr.Name, value)
	}

	return result.Bytes(), nil
}

func checkDiags(diags hcl.Diagnostics) error {
	if diags.HasErrors() {
		for _, diag := range diags {
			if diag.Subject != nil {
				log.Printf("[%s:%d] %s: %s", diag.Subject.Filename, diag.Subject.Start.Line, diag.Summary, diag.Detail)
			} else {
				log.Printf("%s: %s", diag.Summary, diag.Detail)
			}
		}
		return diags
	}

	return nil
}

func findContextsInUse(attrs hcl.Attributes) ([]string, error) {
	trav := []hcl.Traversal{}

	for _, attr := range attrs {
		vars := attr.Expr.Variables()
		if len(vars) == 0 {
			continue
		}

		trav = append(trav, vars...)
	}

	contexts := []string{}

	for _, t := range trav {
		if t.RootName() != "context" {
			return nil, errors.Errorf("Does not support variables other than context")
		}

		expr := hclwrite.NewExpressionAbsTraversal(t)
		tokens := expr.BuildTokens(nil)
		name := string(tokens[2].Bytes)

		exists := false

		for _, ctx := range contexts {
			if ctx == name {
				exists = true
			}
		}

		if !exists {
			contexts = append(contexts, name)
		}
	}

	return contexts, nil
}

func unmarshalFile(fn string) (cty.Value, error) {
	fn = filepath.Clean(fn)

	_, err := os.Lstat(fn)
	if err != nil {
		return cty.NilVal, errors.Wrapf(err, "Failed to stat %q", fn)
	}

	src, err := ioutil.ReadFile(fn)
	if err != nil {
		return cty.NilVal, errors.Wrapf(err, "Failed to read file %v", fn)
	}

	ctype, err := json.ImpliedType(src)
	if err != nil {
		return cty.NilVal, err
	}

	return json.Unmarshal(src, ctype)
}

func readContextFiles(folder string, name string) cty.Value {
	files := []string{
		filepath.Join(folder, fmt.Sprintf(contextFileName, name)),
		filepath.Join(folder, fmt.Sprintf(contextSecretsFileName, name)),
	}

	variables := map[string]cty.Value{}

	for _, file := range files {
		vars, err := unmarshalFile(file)
		if err != nil {
			log.Debugf("Context file %s not found, skipping", file)
			continue
		}

		for k, v := range vars.AsValueMap() {
			variables[k] = v
		}
	}

	return cty.ObjectVal(variables)
}
