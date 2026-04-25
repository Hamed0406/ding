package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ding/ding/internal/alert"
	"github.com/ding/ding/internal/diff"
	"github.com/ding/ding/internal/scanner"
	"github.com/ding/ding/internal/storage"
)

func main() {
	cfg := configFromEnv()

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

func configFromEnv() config {
	return config{
		iface:     envOr("DING_INTERFACE", "eth0"),
		subnet:    envOr("DING_SUBNET", "192.168.1.0/24"),
		ports:     envOr("DING_PORTS", "22,80,443,8080,8443"),
		timeoutMs: envInt("DING_TIMEOUT_MS", 500),
		dataPath:  envOr("DING_DATA_PATH", "/data/ding.json"),
		alert: alert.Config{
			TelegramToken:  os.Getenv("DING_TELEGRAM_TOKEN"),
			TelegramChatID: os.Getenv("DING_TELEGRAM_CHAT_ID"),
		},
	}
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
