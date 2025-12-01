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

	// attempt to open the browser to the home page after server is listening
	var openOnce sync.Once
	go func() {
		// give the server a moment to finish its startup work before opening the browser
		time.Sleep(3 * time.Second)
		openOnce.Do(func() {
			url := "http://localhost" + addr
			_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url+"/home").Start()
		})
	}()

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
