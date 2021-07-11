// Copyright (c) 2T Tech. All rights reserved.
// Licensed under the Apache license.

package main

import (
	"os"

	"github.com/2ttech/spacectx/cmd"
	colorable "github.com/mattn/go-colorable"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetFormatter(&log.TextFormatter{ForceColors: true})
	log.SetOutput(colorable.NewColorableStdout())

	if err := cmd.NewRootCmd().Execute(); err != nil {
		log.Error(err)
		os.Exit(1)
	}
}
