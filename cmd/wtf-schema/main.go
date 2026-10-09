// wtf-schema is a developer generator, not a workspace operation.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/cli"
	"github.com/AndrewPBerg/wtf/internal/core"
)

func generate(path string) error {
	root := cli.New(core.New("", "", io.Discard), io.Discard, io.Discard).Root
	data, err := cli.ConfigurationSchema(root)
	if err != nil {
		return err
	}
	if path == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".schema-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0644); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

func main() {
	output := flag.String("output", "-", "Output schema path; - writes to stdout")
	flag.Parse()
	if err := generate(*output); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
