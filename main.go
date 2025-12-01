package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"mu/admin"
	"mu/api"
	"mu/app"
	"mu/auth"
	"mu/blog"
	"mu/chat"
	"mu/config"
	"mu/data"
	"mu/home"
	"mu/news"
	"mu/user"
	"mu/video"
)

var EnvFlag = flag.String("env", "dev", "Set the environment")
var ServeFlag = flag.Bool("serve", false, "Run the server")
var AddressFlag = flag.String("address", ":8030", "Address for server")

var loadingPage = []byte(app.RenderHTML(
	"Loading",
	"Preparing Mu",
	`<div style="padding: 40px; text-align: center;">
      <h2>Loading...</h2>
      <p>Fetching links and generating vectors. The app will open once everything is ready.</p>
    </div>`,
))

func normalizeAddress(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	if strings.Contains(addr, ":") {
		return addr
	}
	return ":" + addr
}

func main() {
	flag.Parse()

	if !*ServeFlag {
		fmt.Println("--serve not set")
		os.Exit(1)
	}

	// render the api markdwon
	md := api.Markdown()
	apiDoc := app.Render([]byte(md))
	apiHTML := app.RenderHTML("API", "API documentation", string(apiDoc))

	// load the data index
	data.Load()

	// load mutable settings
	config.Load()

	// load admin/flags
	admin.Load()

	// load the chat
	chat.Load()

	// load the news
	news.Load()

	// load the videos
	video.Load()

	// load the blog
	blog.Load()

	// load the home cards
	home.Load()

	// Track readiness so we don't open the app until background indexing completes
	var appReady atomic.Bool
	appReady.Store(false)
	startupReady := make(chan struct{})

	go func() {
		ctx := context.Background()
		waiters := []func(context.Context) error{
			news.WaitReady,
			video.WaitReady,
		}

		for _, wait := range waiters {
			if err := wait(ctx); err != nil {
				fmt.Printf("Startup wait error: %v\n", err)
			}
		}

		appReady.Store(true)
		close(startupReady)
	}()

	// serve video
	http.HandleFunc("/video", video.Handler)

	// serve news
	http.HandleFunc("/news", news.Handler)

	// serve chat
	http.HandleFunc("/chat", chat.Handler)

	// serve blog (full list)
	http.HandleFunc("/posts", blog.Handler)

	// serve individual blog post (public, no auth)
	http.HandleFunc("/post", blog.PostHandler)

	// edit blog post
	http.HandleFunc("/post/edit", blog.EditHandler)

	// flag content
	http.HandleFunc("/flag", admin.FlagHandler)

	// moderation queue
	http.HandleFunc("/moderate", admin.ModerateHandler)

	// admin user management
	http.HandleFunc("/admin", admin.AdminHandler)

	// membership page (public - handles GoCardless redirects)
	http.HandleFunc("/membership", app.Membership)

	// donate page (public - handles GoCardless redirects)
	http.HandleFunc("/donate", app.Donate)

	// serve the home screen
	http.HandleFunc("/home", home.Handler)

	http.HandleFunc("/mail", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/home", 302)
	})

	http.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://coinmarketcap.com/", 302)
	})

	// auth
	http.HandleFunc("/login", app.Login)
	http.HandleFunc("/logout", app.Logout)
	http.HandleFunc("/signup", app.Signup)
	http.HandleFunc("/account", app.Account)
	http.HandleFunc("/session", app.Session)
	http.HandleFunc("/settings", app.Settings)

	// presence ping endpoint
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		username := "guest"
		if sess, err := auth.GetSession(r); err == nil {
			if acc, err := auth.GetAccount(sess.Account); err == nil {
				username = acc.ID
			}
		}
		auth.UpdatePresence(username)

		w.Header().Set("Content-Type", "application/json")
		onlineCount := auth.GetOnlineCount()
		w.Write([]byte(fmt.Sprintf(`{"status":"ok","online":%d}`, onlineCount)))
	})

	// serve the api doc
	http.Handle("/api", app.ServeHTML(apiHTML))

	// serve the app, redirecting "/" to /home
	appHandler := app.Serve()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/home", http.StatusFound)
			return
		}
		appHandler.ServeHTTP(w, r)
	})

	addr := normalizeAddress(*AddressFlag)
	fmt.Println("Starting server on", addr)

	// attempt to open the browser to the home page after background prep finishes
	var openOnce sync.Once

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if *EnvFlag == "dev" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		if !appReady.Load() && !isStaticAsset(r.URL.Path) {
			w.Header().Set("Retry-After", "10")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write(loadingPage)
			return
		}

		// Check if this is a user profile request (/@username)
		if strings.HasPrefix(r.URL.Path, "/@") && !strings.Contains(r.URL.Path[2:], "/") {
			user.Profile(w, r)
			return
		}

		http.DefaultServeMux.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	go func() {
		<-startupReady
		openOnce.Do(func() {
			url := "http://localhost" + addr
			_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url+"/home").Start()
		})
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	fmt.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("Server forced to shutdown: %v\n", err)
	}
}

// isStaticAsset returns true for requests that should bypass the loading gate
// (CSS, JS, icons, manifest, and cached JSON blobs).
func isStaticAsset(path string) bool {
	path = strings.ToLower(path)
	switch {
	case strings.HasSuffix(path, ".css"),
		strings.HasSuffix(path, ".js"),
		strings.HasSuffix(path, ".png"),
		strings.HasSuffix(path, ".jpg"),
		strings.HasSuffix(path, ".jpeg"),
		strings.HasSuffix(path, ".gif"),
		strings.HasSuffix(path, ".svg"),
		strings.HasSuffix(path, ".ico"),
		strings.HasSuffix(path, ".webmanifest"),
		strings.HasSuffix(path, ".json"):
		return true
	}
	return false
}
