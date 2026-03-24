package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Sultan1993/forge/internal/api"
	"github.com/Sultan1993/forge/internal/config"
	"github.com/Sultan1993/forge/internal/system"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "uninstall" {
		fmt.Println("Uninstalling Forge...")
		fmt.Println("Note: Tailscale, SSH, and sleep settings will not be changed.")
		if err := system.Uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		fmt.Println("Forge has been uninstalled.")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Start background CPU sampling
	system.StartCPUMonitor()

	api.SetVersion(version)
	api.SetUpdateRepo("Sultan1993/forge")

	bindAddr := resolveBindAddr(cfg.Port)
	log.Printf("forge %s starting on %s", version, bindAddr)

	platform := system.NewPlatform()
	handler := api.NewRouter(platform)

	srv := &http.Server{
		Addr:         bindAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGTERM/SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	log.Printf("forge is ready")

	<-ctx.Done()
	log.Printf("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}

	log.Printf("stopped")
}

// resolveBindAddr finds the Tailscale interface IP (100.x.x.x) and returns
// "ip:port". Falls back to "0.0.0.0:port" for development.
func resolveBindAddr(port int) string {
	if ip := tailscaleIP(); ip != "" {
		return fmt.Sprintf("%s:%d", ip, port)
	}
	log.Printf("warning: no Tailscale interface found, binding to 0.0.0.0")
	return fmt.Sprintf("0.0.0.0:%d", port)
}

func tailscaleIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil {
				continue
			}
			if strings.HasPrefix(ip.String(), "100.") {
				return ip.String()
			}
		}
	}

	// Fallback: try reading from tailscale CLI
	return ""
}
