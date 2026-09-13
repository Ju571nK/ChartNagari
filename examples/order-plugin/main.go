// Copy this command into a separate Go module and replace simulator.Open with
// your Broker implementation. Only public pkg imports are required.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
	"github.com/Ju571nK/Chatter/pkg/brokerplugin/simulator"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "examples/order-plugin/config.local.yaml", "private YAML configuration")
	flag.Parse()
	f, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer f.Close()
	var cfg struct {
		ID      string `yaml:"plugin_id"`
		Secret  string `yaml:"plugin_secret"`
		Listen  string `yaml:"listen"`
		Journal string `yaml:"journal"`
		State   string `yaml:"state"`
		Enabled bool   `yaml:"orders_enabled"`
	}
	d := yaml.NewDecoder(f)
	d.KnownFields(true)
	if err = d.Decode(&cfg); err != nil {
		return err
	}
	if len(cfg.Secret) < 32 {
		return fmt.Errorf("plugin_secret must contain at least 32 characters")
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:9200"
	}
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("template listener must be loopback; use an authenticated HTTPS reverse proxy for remote access")
	}
	b, err := simulator.Open(cfg.ID, cfg.State)
	if err != nil {
		return err
	}
	defer b.Close()
	j, err := plugin.OpenJournal(cfg.Journal)
	if err != nil {
		return err
	}
	defer j.Close()
	h, err := plugin.NewServer(plugin.ServerConfig{PluginID: cfg.ID, Secret: cfg.Secret, OrdersEnabled: cfg.Enabled}, b, j)
	if err != nil {
		return err
	}
	s := &http.Server{Addr: cfg.Listen, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 25 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopped := make(chan error, 1)
	go func() { stopped <- s.ListenAndServe() }()
	fmt.Printf("Simulator only: %s; order API enabled: %t\n", cfg.Listen, cfg.Enabled)
	select {
	case err = <-stopped:
		if err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		return s.Shutdown(shutdown)
	}
}
