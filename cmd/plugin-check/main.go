// plugin-check only inspects authenticated metadata and account/order snapshots.
// Destructive conformance scenarios run in Go tests against the local simulator.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
	"github.com/Ju571nK/Chatter/pkg/brokerplugin/conformance"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "plugin-check.local.yaml", "private YAML connection file")
	flag.Parse()
	f, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer f.Close()
	var cfg struct {
		URL    string `yaml:"url"`
		ID     string `yaml:"plugin_id"`
		Secret string `yaml:"plugin_secret"`
	}
	d := yaml.NewDecoder(f)
	d.KnownFields(true)
	if err = d.Decode(&cfg); err != nil {
		return err
	}
	c, err := plugin.NewClient(cfg.URL, cfg.ID, cfg.Secret)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := conformance.Inspect(ctx, c)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(r)
}
