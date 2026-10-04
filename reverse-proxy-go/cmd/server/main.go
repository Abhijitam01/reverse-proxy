package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"

	"reverse-proxy-go/internal/config"
	"reverse-proxy-go/internal/handler"
	"reverse-proxy-go/internal/health"
	"reverse-proxy-go/internal/router"
)

func main() {
	configFile := flag.String("config", "config.json", "Path to configuration file")
	flag.Parse()

	proxyConfig, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	healthChecker := health.NewHealthChecker()
	defer healthChecker.Close()

	r := router.NewRouter(healthChecker)
	r.SetRoutes(proxyConfig.Routes)

	h := handler.NewHandler(r, healthChecker)

	mux := chi.NewRouter()

	mux.Get("/_proxy/routes", h.ListRoutes)
	mux.Post("/_proxy/routes", h.AddRoute)
	mux.Delete("/_proxy/routes/{index}", h.RemoveRoute)
	mux.Get("/_proxy/health", h.Health)

	mux.HandleFunc("/*", h.ServeProxy)

	server := &http.Server{
		Addr:    proxyConfig.ListenAddr,
		Handler: mux,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down server...")
		healthChecker.Close()
		server.Close()
		os.Exit(0)
	}()

	log.Printf("Starting reverse proxy on %s with %d routes", proxyConfig.ListenAddr, len(proxyConfig.Routes))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
