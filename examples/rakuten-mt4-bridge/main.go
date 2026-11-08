// Rakuten FX has no official REST API. This process is a loopback
// placeholder. It serves the v1 order-plugin protocol from the local
// simulator and does not open a broker connection.
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
	path := flag.String("config", "examples/rakuten-mt4-bridge/config.local.yaml", "private YAML configuration")
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
		Mode    string `yaml:"bridge_mode"`
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
	if cfg.Enabled {
		return fmt.Errorf("orders_enabled must be false until an MT4 transport exists")
	}
	if cfg.Mode != "not-connected" {
		return fmt.Errorf("bridge_mode must be not-connected (MT4 EA has no transport in this build)")
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:9300"
	}
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("listener must be loopback; use an authenticated HTTPS reverse proxy for remote access")
	}
	sim, err := simulator.Open(cfg.ID, cfg.State)
	if err != nil {
		return err
	}
	defer sim.Close()
	j, err := plugin.OpenJournal(cfg.Journal)
	if err != nil {
		return err
	}
	defer j.Close()
	h, err := plugin.NewServer(plugin.ServerConfig{PluginID: cfg.ID, Secret: cfg.Secret, OrdersEnabled: false}, bridge{Broker: sim}, j)
	if err != nil {
		return err
	}
	s := &http.Server{Addr: cfg.Listen, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 25 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopped := make(chan error, 1)
	go func() { stopped <- s.ListenAndServe() }()
	fmt.Printf("Rakuten MT4 bridge placeholder on %s; bridge_mode %s; orders_enabled false\n", cfg.Listen, cfg.Mode)
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

// bridge keeps the simulator for local metadata queries and replaces the
// public identity. Submit and cancel stay off so inspection cannot read this
// process as an order route.
type bridge struct {
	*simulator.Broker
}

func (b bridge) Manifest() plugin.Manifest {
	m := b.Broker.Manifest()
	m.Name = "Rakuten MT4 bridge"
	m.Version = "0.1.0"
	m.Capabilities.Submit = false
	m.Capabilities.Cancel = false
	m.Settings = append(m.Settings, plugin.SettingField{
		Key:      "bridge_mode",
		Label:    "Bridge mode",
		Type:     "select",
		Required: true,
		Options:  []string{"not-connected", "MT4 EA"},
	})
	return m
}
