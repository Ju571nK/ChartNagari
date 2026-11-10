// Command plugin-rakutenfx is a read-only order-plugin v1 adapter for a
// Rakuten Securities MT4 DEMO account. ChartNagariBridge.mq4 (in this
// directory) pushes snapshots to /bridge/v1/snapshot; ChartNagari reads them
// through the signed /v1 API. Orders cannot be placed. See README.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"github.com/Ju571nK/Chatter/internal/plugins/rakutenfx"
	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Str("component", "plugin-rakutenfx").Logger()
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("plugin-rakutenfx stopped")
	}
}

func run() error {
	path := flag.String("config", "cmd/plugin-rakutenfx/config.local.yaml", "private YAML configuration")
	flag.Parse()
	f, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer f.Close()
	var cfg struct {
		ID         string `yaml:"plugin_id"`
		Secret     string `yaml:"plugin_secret"`
		Token      string `yaml:"bridge_token"`
		Listen     string `yaml:"listen"`
		Journal    string `yaml:"journal"`
		DemoServer string `yaml:"demo_server"`
	}
	d := yaml.NewDecoder(f)
	d.KnownFields(true)
	if err = d.Decode(&cfg); err != nil {
		return err
	}
	if len(cfg.Secret) < 32 {
		return errors.New("plugin_secret must contain at least 32 characters")
	}
	if cfg.Token == cfg.Secret {
		return errors.New("bridge_token must differ from plugin_secret")
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:9300"
	}
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return err
	}
	// The EA sends its token over plain HTTP, so it must never leave the machine.
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("listen must be a loopback IP such as 127.0.0.1")
	}
	bridge, err := rakutenfx.New(cfg.ID, cfg.Token, cfg.DemoServer)
	if err != nil {
		return err
	}
	j, err := plugin.OpenJournal(cfg.Journal)
	if err != nil {
		return err
	}
	defer j.Close()
	// OrdersEnabled stays false: the bridge has no order path.
	api, err := plugin.NewServer(plugin.ServerConfig{PluginID: cfg.ID, Secret: cfg.Secret}, bridge, j)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/bridge/", bridge)
	mux.Handle("/", api)
	s := &http.Server{Addr: cfg.Listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 25 * time.Second, IdleTimeout: 60 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopped := make(chan error, 1)
	go func() { stopped <- s.ListenAndServe() }()
	log.Info().Str("listen", cfg.Listen).Str("snapshot_url", "http://"+cfg.Listen+rakutenfx.SnapshotPath).Msg("read-only MT4 demo bridge started")
	select {
	case err = <-stopped:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		return s.Shutdown(shutdown)
	}
}
