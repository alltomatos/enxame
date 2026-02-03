package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/goautomatik/core-server/pkg/client_sdk"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║          🐝 ENXAME DESKTOP CLIENT (V5 - SDK)              ║")
	fmt.Println("║          🎨 Wails Ready Architecture                      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	profile := "default"
	if len(os.Args) > 1 {
		profile = os.Args[1]
	}

	fmt.Printf("Display Name / Profile: %s\n", profile)

	// INIT SDK
	client, err := client_sdk.NewEnxameClient(profile)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	fmt.Printf("\n🔑 Your Node ID: %s\n", client.GetNodeID())
	os.WriteFile(fmt.Sprintf("%s.nodeid", client.GetNodeID()[:8]), []byte(client.GetNodeID()), 0644)

	// REGISTER EVENT CALLBACK (UI UPDATE)
	client.SetMessageCallback(func(evt client_sdk.MessageEvent) {
		if evt.IsChannel {
			fmt.Printf("\n[%s] %s: %s\n> ", evt.TargetID, evt.SenderID[:8], evt.Content)
		} else {
			fmt.Printf("\n🔒 P2P [%s]: %s\n> ", evt.SenderID[:8], evt.Content)
		}
	})

	// CONNECT
	if err := client.Initialize(); err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}

	fmt.Println("\n✅ Connected! Type /help for commands.")

	// INPUT LOOP
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Print("> ")
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				fmt.Print("> ")
				continue
			}

			handleInput(client, line)
			fmt.Print("> ")
		}
	}()

	<-sigChan
	fmt.Println("\n👋 Shutting down...")
}

func handleInput(client *client_sdk.EnxameClient, line string) {
	if strings.HasPrefix(line, "/") {
		parts := strings.Fields(line)
		cmd := parts[0]

		switch cmd {
		case "/join":
			if len(parts) < 2 {
				fmt.Println("Usage: /join <#channel>")
				return
			}
			if err := client.JoinChannel(parts[1]); err != nil {
				fmt.Printf("Error joining: %v\n", err)
			} else {
				fmt.Printf("Joined %s\n", parts[1])
				// history?
				msgs, _ := client.GetHistory(parts[1])
				for _, m := range msgs {
					fmt.Printf("HIST [%s] %s: %s\n", m.Timestamp.Format("15:04"), m.SenderID[:8], m.Content)
				}
			}

		case "/create":
			if len(parts) < 2 {
				return
			}
			id, err := client.CreateChannel(parts[1])
			if err != nil {
				fmt.Printf("Error creating: %v\n", err)
			} else {
				fmt.Printf("Channel %s created!\n", id)
				client.JoinChannel(id)
			}

		case "/to":
			// /to NodeID msg...
			if len(parts) < 3 {
				return
			}
			client.SendMessage(parts[1], strings.Join(parts[2:], " "))

		case "/promote":
			if len(parts) < 4 {
				fmt.Println("Usage: /promote <NodeID> <Role> <#channel>")
				return
			}
			client.PromoteUser(parts[1], parts[2], parts[3])

		case "/demote":
			if len(parts) < 3 {
				fmt.Println("Usage: /demote <NodeID> <#channel>")
				return
			}
			client.DemoteUser(parts[1], parts[2])

		case "/kick":
			if len(parts) < 3 {
				fmt.Println("Usage: /kick <NodeID> <#channel>")
				return
			}
			client.KickUser(parts[1], parts[2])

		case "/quit", "/exit":
			os.Exit(0)

		default:
			fmt.Println("Unknown command")
		}
	} else {
		// Chat to current channel? SDK doesn't track "current UI context", the UI should.
		// NOTE: In this CLI refactor, I'm simplifying. I'll force user to use /to or /chat (or I need to track context in CLI).
		// Let's add context tracking in CLI to match previous behavior.
		fmt.Println("⚠️ Please use /to <ID> <msg> or /join <chan> then send (Not fully implemented in simple CLI view)")
	}
}
