// Command testserver runs an isolated development server for browser smoke tests.
// It deliberately uses a nonexistent core path; UI tests must display pending state.
package main

import (
	"context"
	"fmt"
	"github.com/uniproxy/uniproxy/internal/app"
	"github.com/uniproxy/uniproxy/internal/core"
	"github.com/uniproxy/uniproxy/internal/store"
	"github.com/uniproxy/uniproxy/internal/webui"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

func main() {
	dir, e := os.MkdirTemp("", "uniproxy-browser-test-")
	if e != nil {
		log.Fatal(e)
	}
	defer os.RemoveAll(dir)
	s, e := store.Open(dir)
	if e != nil {
		log.Fatal(e)
	}
	defer s.Close()
	m := core.New(core.Options{BinaryPath: filepath.Join(dir, "not-installed"), DataDir: filepath.Join(dir, "core")})
	a := app.New(s, m, app.Options{Host: "127.0.0.1", AdminPort: 19090})
	a.Start()
	defer a.Close()
	mux := http.NewServeMux()
	mux.Handle("/api/", a.Handler())
	mux.Handle("/", webui.Handler())
	server := http.Server{Addr: "127.0.0.1:19090", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		fmt.Println("isolated browser test http://127.0.0.1:19090")
		if e := server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Print(e)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}
