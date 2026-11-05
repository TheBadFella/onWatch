package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("agy-bridge failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("agy-bridge", flag.ContinueOnError)
	port := fs.Int("port", 0, "listen port on 127.0.0.1 (0 picks a free port)")
	accountName := fs.String("account-name", "", "label for the account this bridge serves")
	profileDir := fs.String("profile-dir", "", "isolated profile directory for this account's agy credentials")
	csrfToken := fs.String("csrf-token", "", "CSRF token clients must send (generated if empty)")
	agyPath := fs.String("agy-path", "", "explicit path to the agy binary")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	token := strings.TrimSpace(*csrfToken)
	if token == "" {
		generated, err := generateCSRFToken()
		if err != nil {
			return fmt.Errorf("generate CSRF token: %w", err)
		}
		token = generated
	}

	var opts []api.AntigravityCLIRunnerOption
	if path := strings.TrimSpace(*agyPath); path != "" {
		opts = append(opts, api.WithAgyBinaryPath(path))
	}
	profile := strings.TrimSpace(*profileDir)
	if profile != "" {
		if err := os.MkdirAll(profile, 0o700); err != nil {
			return fmt.Errorf("create profile dir: %w", err)
		}
		opts = append(opts, api.WithAgyLaunchEnv(profileDirEnv(profile)))
	}

	runner := api.NewAntigravityCLIRunner(logger, opts...)
	defer runner.Stop()

	b := newBridge(runner, token, logger)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	srv := &http.Server{
		Handler:           b.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	baseURL := "http://" + ln.Addr().String()
	logger.Info("agy-bridge listening",
		"account", strings.TrimSpace(*accountName),
		"base_url", baseURL,
		"profile_dir", profile,
		"version", version,
	)
	// The generated token is required to configure onWatch, so print it once.
	fmt.Fprintf(os.Stderr, "agy-bridge base_url: %s\nagy-bridge csrf_token: %s\n", baseURL, token)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		logger.Info("agy-bridge shutting down")
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	runner.Stop()
	return nil
}

func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
