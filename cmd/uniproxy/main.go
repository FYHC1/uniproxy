package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/uniproxy/uniproxy/internal/app"
	"github.com/uniproxy/uniproxy/internal/appdata"
	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/processguard"
	"github.com/uniproxy/uniproxy/internal/store"
	"github.com/uniproxy/uniproxy/internal/version"
	"github.com/uniproxy/uniproxy/internal/webui"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var errCLIUsage = errors.New("invalid command-line arguments; use -h for supported flags")

func failureExitCode(err error) int {
	if errors.Is(err, errCLIUsage) {
		return 2
	}
	return 1
}

type options struct {
	host, data, binary, token string
	port                      int
	showVersion, healthcheck  bool
}

func parseOptions(args []string, output io.Writer) (options, error) {
	var opts options
	portDefault, err := strconv.Atoi(env("UNIPROXY_PORT", "9090"))
	if err != nil {
		return opts, errors.New("UNIPROXY_PORT must be a valid port")
	}
	flags := flag.NewFlagSet("uniproxy", flag.ContinueOnError)
	// Invalid flag values can contain pasted credentials. Do not copy raw
	// arguments into durable diagnostics; help is printed explicitly below.
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.host, "host", env("UNIPROXY_HOST", "127.0.0.1"), "admin listen IP")
	flags.IntVar(&opts.port, "port", portDefault, "admin listen port")
	flags.StringVar(&opts.data, "data", env("UNIPROXY_DATA_DIR", ""), "persistent data directory (default: platform standard location)")
	flags.StringVar(&opts.binary, "core", env("UNIPROXY_CORE_PATH", ""), "Mihomo executable path")
	flags.BoolVar(&opts.showVersion, "version", false, "show version")
	flags.BoolVar(&opts.healthcheck, "healthcheck", false, "check running local service health")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(output)
			fmt.Fprintln(output, "Usage: uniproxy [options]")
			flags.PrintDefaults()
			return opts, flag.ErrHelp
		}
		return opts, errCLIUsage
	}
	if flags.NArg() != 0 {
		return opts, errors.New("unexpected positional arguments; use -h for supported flags")
	}
	if opts.showVersion {
		return opts, nil
	}
	if opts.port < 1024 || opts.port > 65535 {
		return opts, errors.New("admin port must be 1024..65535")
	}
	if opts.healthcheck {
		return opts, nil
	}
	ip := net.ParseIP(opts.host)
	if ip == nil {
		return opts, errors.New("host must be an IP literal")
	}
	opts.token = os.Getenv("UNIPROXY_ADMIN_TOKEN")
	if opts.token != "" && len(opts.token) < 24 {
		return opts, errors.New("admin token must contain at least 24 characters")
	}
	if !ip.IsLoopback() && opts.token == "" {
		return opts, errors.New("non-loopback admin binding requires UNIPROXY_ADMIN_TOKEN (24+ characters)")
	}
	return opts, nil
}

// Failed health probes are expected during service restarts; monitors may run
// every few seconds, so they must not create an unbounded series of disk logs.
type healthcheckFailure struct{ error }

func (e *healthcheckFailure) Unwrap() error { return e.error }

func run(ctx context.Context, args []string, output io.Writer, logger *log.Logger) (result error) {
	opts, err := parseOptions(args, output)
	if opts.healthcheck && !opts.showVersion {
		defer func() {
			if result != nil {
				result = &healthcheckFailure{result}
			}
		}()
	}
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if opts.showVersion {
		fmt.Fprintln(output, "UniProxy "+version.String())
		return nil
	}
	if opts.healthcheck {
		client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}
		defer client.CloseIdleConnections()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/health", opts.port), nil)
		if err != nil {
			return err
		}
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return errors.New("healthcheck failed")
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Establish Windows child ownership before even the first version probe.
	// The guard is process-lifetime, not something a runtime Stop can close.
	if err := processguard.Enable(); err != nil {
		return fmt.Errorf("initialize child-process protection: %w", err)
	}
	return serve(ctx, opts, logger)
}

func serve(ctx context.Context, opts options, logger *log.Logger) error {
	logger.Printf("UniProxy %s", version.String())
	address := net.JoinHostPort(opts.host, strconv.Itoa(opts.port))
	// A second double-click must fail before opening/writing the existing store
	// or starting another core. Never kill an unrelated owner or change ports.
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("admin listen %s: %w；请检查是否已有 UniProxy 或其他程序占用该管理端口", address, err)
	}
	defer listener.Close()
	resolved, err := appdata.Resolve(opts.data, appdata.Default())
	if err != nil {
		return fmt.Errorf("resolve persistent data directory: %w", err)
	}
	dataDir := resolved.Dir
	// Report the actual path BEFORE store.Open, so an ACL/key/SQLite error is
	// diagnosable even when the web UI and activity store cannot be opened.
	logger.Printf("数据目录: %s (%s)", dataDir, resolved.Source)
	if resolved.Note != "" {
		logger.Print(resolved.Note)
	}
	s, err := store.Open(dataDir)
	if err != nil {
		return fmt.Errorf("open persistent data directory %q: %w；请保留数据库、WAL/SHM 和原 master.key，不要删除数据或换用空目录", dataDir, err)
	}
	defer s.Close()
	manager := core.New(core.Options{BinaryPath: opts.binary, DataDir: filepath.Join(dataDir, "core"), Log: func(level, message string) { s.Activity(level, message) }})
	appOpts := app.Options{Token: opts.token, Host: opts.host, AdminPort: opts.port, AllowPrivateSubscriptions: os.Getenv("UNIPROXY_ALLOW_PRIVATE_SUBSCRIPTIONS") == "true", Version: version.Version}
	for _, v := range strings.Split(os.Getenv("UNIPROXY_ALLOWED_ORIGINS"), ",") {
		if v = strings.TrimSpace(v); v != "" {
			appOpts.AllowedOrigins = append(appOpts.AllowedOrigins, v)
		}
	}
	application := app.New(s, manager, appOpts)
	// Register cancellation before Start, including a blocked cold core probe.
	cleanup := watchApplication(ctx, application)
	defer cleanup()
	application.Start()
	if ctx.Err() != nil {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", application.Handler())
	mux.Handle("/", webui.Handler())
	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32768, BaseContext: func(net.Listener) context.Context { return ctx }}
	defer server.Close()
	errorsCh := make(chan error, 1)
	go func() { errorsCh <- server.Serve(listener) }()
	logger.Printf("UniProxy %s listening on http://%s", version.String(), address)
	if opts.token == "" {
		logger.Print("Admin authentication disabled for loopback-only access. Set UNIPROXY_ADMIN_TOKEN to enable.")
	}
	select {
	case <-ctx.Done():
	case err := <-errorsCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}

// App.Close cancels its background context BEFORE waiting for lifecycle locks.
// Keep that cancellation independent of HTTP draining and blocked Start work.
func watchApplication(ctx context.Context, application interface{ Close() }) func() {
	var closeOnce, cleanupOnce sync.Once
	closeApp := func() { closeOnce.Do(application.Close) }
	watchDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			closeApp()
		case <-watchDone:
		}
	}()
	return func() {
		cleanupOnce.Do(func() { close(watchDone); closeApp() })
	}
}

func mainExitCode() int {
	// Capture launch mode BEFORE core children attach to this console. Only a
	// no-argument, standalone interactive Windows console may wait on failure.
	holdConsole := len(os.Args) == 1 && standaloneConsole()
	if holdConsole {
		prepareConsole()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	report := &diagnosticBuffer{}
	logger := log.New(io.MultiWriter(report, os.Stderr), "", log.LstdFlags)
	if err := run(ctx, os.Args[1:], os.Stdout, logger); err != nil {
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return 0
		}
		logger.Printf("UniProxy %s 启动/运行失败: %v", version.String(), err)
		var probe *healthcheckFailure
		if !errors.As(err, &probe) {
			if path, saveErr := saveStartupReport(report.Bytes()); saveErr == nil {
				fmt.Fprintf(os.Stderr, "错误日志: %s\n", path)
			} else {
				fmt.Fprintln(os.Stderr, "无法保存错误日志；请保留上方错误文字。")
			}
		}
		if holdConsole && ctx.Err() == nil {
			waitForAcknowledgement(ctx, os.Stdin, os.Stderr)
		}
		return failureExitCode(err)
	}
	return 0
}

func main() { os.Exit(mainExitCode()) }
