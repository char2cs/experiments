// Command mcserver is a Minecraft-protocol test server for exercising Quiver
// end to end: it serves what a server-list ping and the query protocol ask
// for, with its identity taken from the arrow's variables and ports.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/char2cs/experiments/internal/mcproto"
	"github.com/char2cs/experiments/internal/server"
)

// version is stamped by the release build.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "mcserver:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: mcserver serve|ping|query|version")
	}
	switch args[0] {
	case "serve":
		return serve(ctx, args[1:], stderr)
	case "ping":
		return ping(ctx, args[1:], stdout)
	case "query":
		return query(ctx, args[1:], stdout)
	case "version":
		fmt.Fprintln(stdout, version)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func serve(ctx context.Context, args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := server.Config{}
	level := fs.String("log-level", "info", "info or debug")
	fs.StringVar(&cfg.Host, "host", "0.0.0.0", "address to bind")
	fs.IntVar(&cfg.Port, "port", 25565, "TCP game port")
	fs.IntVar(&cfg.QueryPort, "query-port", 0, "UDP query port, 0 disables it")
	fs.StringVar(&cfg.MOTD, "motd", "A Quiver test server", "message of the day")
	fs.IntVar(&cfg.MaxPlayers, "max-players", 20, "advertised player slots")
	fs.StringVar(&cfg.VersionName, "version-name", "Quiver 1.21", "advertised version name")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	var lv slog.Level
	switch *level {
	case "info":
		lv = slog.LevelInfo
	case "debug":
		lv = slog.LevelDebug
	default:
		return fmt.Errorf("log level %q: want info or debug", *level)
	}
	cfg.Logger = slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: lv}))

	srv, err := server.Listen(cfg)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if err := srv.Serve(ctx); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	cfg.Logger.InfoContext(ctx, "stopped")
	return nil
}

func ping(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	host := fs.String("host", "127.0.0.1", "server host")
	port := fs.Int("port", 25565, "server TCP port")
	timeout := fs.Duration("timeout", 5*time.Second, "overall timeout")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	st, rtt, err := mcproto.Ping(ctx, fmt.Sprintf("%s:%d", *host, *port))
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	fmt.Fprintf(stdout, "%s | %s | players %d/%d | %s\n", st.Description.Text, st.Version.Name, st.Players.Online, st.Players.Max, rtt.Round(time.Microsecond))
	return nil
}

func query(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	host := fs.String("host", "127.0.0.1", "server host")
	port := fs.Int("port", 25566, "server UDP query port")
	timeout := fs.Duration("timeout", 5*time.Second, "overall timeout")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	info, err := mcproto.Query(ctx, fmt.Sprintf("%s:%d", *host, *port))
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	fmt.Fprintf(stdout, "%s | %s | players %d/%d | game port %d\n", info.MOTD, info.GameType, info.NumPlayers, info.MaxPlayers, info.HostPort)
	return nil
}
