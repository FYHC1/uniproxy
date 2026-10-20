package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/uniproxy/uniproxy/internal/app"
	"github.com/uniproxy/uniproxy/internal/appdata"
	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/store"
	"github.com/uniproxy/uniproxy/internal/version"
	"github.com/uniproxy/uniproxy/internal/webui"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func run() error {
	host := flag.String("host", env("UNIPROXY_HOST", "127.0.0.1"), "admin listen IP")
	portDefault, e := strconv.Atoi(env("UNIPROXY_PORT", "9090"))
	if e != nil {
		return errors.New("UNIPROXY_PORT must be a valid port")
	}
	port := flag.Int("port", portDefault, "admin listen port")
	// Empty means "use the platform standard location"; see internal/appdata.
	dataFlag := flag.String("data", env("UNIPROXY_DATA_DIR", ""), "persistent data directory (default: platform standard location)")
	binary := flag.String("core", env("UNIPROXY_CORE_PATH", ""), "Mihomo executable path")
	showVersion := flag.Bool("version", false, "show version")
	healthcheck := flag.Bool("healthcheck", false, "check running local service health")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments; use -h for supported flags")
	}
	if *showVersion {
		fmt.Println("UniProxy " + version.String())
		return nil
	}
	if *port < 1024 || *port > 65535 {
		return errors.New("admin port must be 1024..65535")
	}
	if *healthcheck {
		c := http.Client{Timeout: 3 * time.Second}
		res, e := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", *port))
		if e != nil {
			return e
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			return errors.New("healthcheck failed")
		}
		return nil
	}
	ip := net.ParseIP(*host)
	if ip == nil {
		return errors.New("host must be an IP literal")
	}
	token := os.Getenv("UNIPROXY_ADMIN_TOKEN")
	if token != "" && len(token) < 24 {
		return errors.New("admin token must contain at least 24 characters")
	}
	if !ip.IsLoopback() && token == "" {
		return errors.New("non-loopback admin binding requires UNIPROXY_ADMIN_TOKEN (24+ characters)")
	}
	resolved, e := appdata.Resolve(*dataFlag, appdata.Default())
	if e != nil {
		return e
	}
	dataDir := resolved.Dir
	if *binary == "" {
		name := "mihomo"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		*binary = filepath.Join(dataDir, "core", name)
		if _, e := os.Stat(*binary); e != nil {
			if _, e := os.Stat(filepath.Join("bin", name)); e == nil {
				*binary = filepath.Join("bin", name)
			}
		}
		if _, e := os.Stat(*binary); e != nil {
			if exe, e := os.Executable(); e == nil {
				candidate := filepath.Join(filepath.Dir(exe), name)
				if _, e := os.Stat(candidate); e == nil {
					*binary = candidate
				}
			}
		}
	}
	s, e := store.Open(dataDir)
	if e != nil {
		return e
	}
	defer s.Close()
	manager := core.New(core.Options{BinaryPath: *binary, DataDir: filepath.Join(dataDir, "core"), Log: func(level, message string) { s.Activity(level, message) }})
	opts := app.Options{Token: token, Host: *host, AdminPort: *port, AllowPrivateSubscriptions: os.Getenv("UNIPROXY_ALLOW_PRIVATE_SUBSCRIPTIONS") == "true", Version: version.Version}
	for _, v := range strings.Split(os.Getenv("UNIPROXY_ALLOWED_ORIGINS"), ",") {
		if v = strings.TrimSpace(v); v != "" {
			opts.AllowedOrigins = append(opts.AllowedOrigins, v)
		}
	}
	application := app.New(s, manager, opts)
	mux := http.NewServeMux()
	mux.Handle("/api/", application.Handler())
	mux.Handle("/", webui.Handler())
	server := &http.Server{Addr: net.JoinHostPort(*host, strconv.Itoa(*port)), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32768}
	listener, e := net.Listen("tcp", server.Addr)
	if e != nil {
		return fmt.Errorf("admin listen: %w", e)
	}
	application.Start()
	defer application.Close()
	errorsCh := make(chan error, 1)
	go func() { errorsCh <- server.Serve(listener) }()
	log.Printf("UniProxy %s listening on http://%s", version.String(), server.Addr)
	log.Printf("数据目录: %s (%s)", dataDir, resolved.Source)
	if resolved.Note != "" {
		log.Print(resolved.Note)
	}
	if token == "" {
		log.Print("Admin authentication disabled for loopback-only access. Set UNIPROXY_ADMIN_TOKEN to enable.")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case e := <-errorsCh:
		if !errors.Is(e, http.ErrServerClosed) {
			return e
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}
func main() {
	if e := run(); e != nil {
		log.Print(e)
		os.Exit(1)
	}
}
