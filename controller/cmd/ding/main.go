package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ding/ding/internal/alert"
	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/iface"
	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
)

func main() {
	cfg, err := configFromEnv()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	store, err := storage.New(cfg.dataPath)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	results, err := scanner.Run(cfg.iface, cfg.subnet, cfg.ports, cfg.timeoutMs)
	if err != nil {
		log.Fatalf("scan: %v", err)
	}

	previous := store.Latest()
	changes := diff.Compare(previous, results)

	if err := store.Save(results); err != nil {
		log.Fatalf("save: %v", err)
	}

	for _, c := range changes {
		fmt.Println(c)
	}
	for _, r := range results {
		mac := "-"
		if r.MAC != nil {
			mac = *r.MAC
		}
		fmt.Printf("%-16s  %-20s  alive=%-5v  ports=%v\n", r.IP, mac, r.Alive, r.OpenPorts)
	}

	if err := alert.Send(cfg.alert, changes); err != nil {
		log.Printf("alert: %v", err)
	}
}

type config struct {
	iface     string
	subnet    string
	ports     string
	timeoutMs int
	dataPath  string
	alert     alert.Config
}

func configFromEnv() (config, error) {
	cfg := config{
		ports:     envOr("DING_PORTS", "22,80,443,8080,8443"),
		timeoutMs: envInt("DING_TIMEOUT_MS", 500),
		dataPath:  envOr("DING_DATA_PATH", "/data/ding.json"),
		alert: alert.Config{
			TelegramToken:  os.Getenv("DING_TELEGRAM_TOKEN"),
			TelegramChatID: os.Getenv("DING_TELEGRAM_CHAT_ID"),
		},
	}

	cfg.iface = os.Getenv("DING_INTERFACE")
	cfg.subnet = os.Getenv("DING_SUBNET")

	if cfg.iface == "" || cfg.subnet == "" {
		// Auto-detect when either is unset
		detected, err := resolveInterface(cfg.iface)
		if err != nil {
			return config{}, err
		}
		if cfg.iface == "" {
			cfg.iface = detected.Name
		}
		if cfg.subnet == "" {
			cfg.subnet = detected.Subnet
		}
		log.Printf("auto-detected interface: %s  subnet: %s", cfg.iface, cfg.subnet)
	}

	return cfg, nil
}

// resolveInterface returns the interface matching hint (if set) or the best available one.
func resolveInterface(hint string) (iface.Interface, error) {
	all, err := iface.All()
	if err != nil {
		return iface.Interface{}, err
	}
	if len(all) == 0 {
		return iface.Interface{}, fmt.Errorf("no usable network interface found")
	}

	if hint != "" {
		for _, i := range all {
			if i.Name == hint {
				return i, nil
			}
		}
		return iface.Interface{}, fmt.Errorf("interface %q not found or has no IPv4 address", hint)
	}

	log.Printf("available interfaces: %v", ifaceNames(all))
	return all[0], nil
}

func ifaceNames(ifaces []iface.Interface) []string {
	names := make([]string, len(ifaces))
	for i, ifc := range ifaces {
		names[i] = fmt.Sprintf("%s(%s)", ifc.Name, ifc.Subnet)
	}
	return names
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}
