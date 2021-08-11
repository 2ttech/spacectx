package helpers

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/pkg/errors"
)

func ReadFiles(fn string) ([]*hclwrite.File, error) {
	if fn == "" {
		fn = "."
	}

	fn = filepath.Clean(fn)

	info, err := os.Lstat(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to stat %q", fn)
	}

	if info.IsDir() {
		if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
			return nil, nil
		}

		return processDir(fn)
	} else {
		f, err := ReadFile(fn)
		if f == nil {
			return []*hclwrite.File{}, err
		}

		return []*hclwrite.File{f}, err
	}
}

func ReadFile(fn string) (*hclwrite.File, error) {
	fn = filepath.Clean(fn)

	info, err := os.Lstat(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to stat %s", fn)
	}

	if info.IsDir() {
		log.Debugf("Skipping %s: it is a directory", fn)
		return nil, nil
	}

	if !info.Mode().IsRegular() {
		log.Debugf("Skipping %s: not a regular file or directory", fn)
		return nil, nil
	}
	if !strings.HasSuffix(fn, ".tf") {
		return nil, nil
	}

	return processFile(fn)
}

func processDir(fn string) ([]*hclwrite.File, error) {
	entries, err := ioutil.ReadDir(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to read directory %s", fn)
	}

	files := []*hclwrite.File{}
	for _, entry := range entries {
		file, err := ReadFile(filepath.Join(fn, entry.Name()))
		if err != nil {
			return nil, err
		}
		if file == nil {
			continue
		}

		files = append(files, file)
	}

	return files, nil
}

func processFile(fn string) (*hclwrite.File, error) {
	src, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to read file %s", fn)
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered in processFile while processing %s: %#v\n%s", fn, r, debug.Stack())
		}
	}()

	log.Debugf("Parsing hcl file %s", fn)
	f, diags := hclwrite.ParseConfig(src, fn, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		for _, diag := range diags {
			if diag.Subject != nil {
				log.Printf("[%s:%d] %s: %s", diag.Subject.Filename, diag.Subject.Start.Line, diag.Summary, diag.Detail)
			} else {
				log.Printf("%s: %s", diag.Summary, diag.Detail)
			}
		}
		return nil, diags
	}

	return f, err
}
