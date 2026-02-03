package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/goautomatik/core-server/pkg/client_sdk"
	"github.com/goautomatik/core-server/pkg/storage"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx context.Context
	sdk *client_sdk.EnxameClient
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown is called at application termination
func (a *App) shutdown(ctx context.Context) {
	if a.sdk != nil {
		a.sdk.Close()
	}
}

// --- Exposed Methods (JS -> Go) ---

// Initialize creates the SDK client and connects
func (a *App) Initialize(profile string) string {
	client, err := client_sdk.NewEnxameClient(profile)
	if err != nil {
		return fmt.Sprintf("Error creating client: %v", err)
	}
	a.sdk = client

	// Event Bridge: SDK -> Wails -> React
	a.sdk.SetMessageCallback(func(evt client_sdk.MessageEvent) {
		runtime.EventsEmit(a.ctx, "message:received", evt)
		log.Printf("[Bridge] Emitted message:received for %s", evt.ID)
	})

	a.sdk.SetGridCallback(func(status, jobID string) {
		runtime.EventsEmit(a.ctx, "grid:status", status)
	})

	// Connect to Network
	if err := a.sdk.Initialize(); err != nil {
		return fmt.Sprintf("Error connecting: %v", err)
	}

	return "" // Success
}

func (a *App) GetNodeID() string {
	if a.sdk == nil {
		return ""
	}
	return a.sdk.GetNodeID()
}

func (a *App) JoinChannel(channel string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}
	if err := a.sdk.JoinChannel(channel); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) GetJoinedChannels() []string {
	if a.sdk == nil {
		return []string{}
	}
	channels, _ := a.sdk.GetJoinedChannels()
	return channels
}

func (a *App) GetRecentChats() []storage.ChatItem {
	if a.sdk == nil {
		return []storage.ChatItem{}
	}
	chats, _ := a.sdk.GetRecentChats()
	return chats
}

func (a *App) StartDM(targetNodeID string) string {
	if a.sdk == nil {
		return ""
	}
	// Logic: Get proper DM ID, ensure storage session exists
	dmID := a.sdk.GetDMChannelID(targetNodeID)
	// Upsert session to ensure it appears in lists
	// We need direct access to storage or helper in sdk.
	// The SDK's SendMessage does upsert, but just clicking "Start DM" should also ensure it exists.
	// For now, let's just return the ID. Actual creation happens on first message OR we expect UI to handle empty chat.
	return dmID
}

func (a *App) GetChannelMembers(channelID string) []client_sdk.Member {
	if a.sdk == nil {
		return []client_sdk.Member{}
	}
	members, _ := a.sdk.GetChannelMembers(channelID)
	return members
}

// SendMessage sends a text message
func (a *App) SendMessage(targetID, message string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}
	if err := a.sdk.SendMessage(targetID, message); err != nil {
		return err.Error()
	}
	return ""
}

// SendFile sends a file
func (a *App) SendFile(targetID, filePath string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}
	if err := a.sdk.SendFile(targetID, filePath); err != nil {
		return err.Error()
	}
	return ""
}

// DownloadAttachment opens a dialog to save the file
func (a *App) DownloadAttachment(fileID string, originalName string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}

	dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: originalName,
		Title:           "Save File",
		Filters: []runtime.FileFilter{
			{DisplayName: "All Files", Pattern: "*.*"},
		},
	})

	if err != nil {
		return err.Error()
	}
	if dest == "" {
		return "cancelled"
	}

	if err := a.sdk.SaveFileToDisk(fileID, dest); err != nil {
		return err.Error()
	}
	return "success"
}

func (a *App) GetHistory(target string) []storage.Message {
	if a.sdk == nil {
		return nil
	}
	msgs, _ := a.sdk.GetHistory(target)
	return msgs
}

func (a *App) PromoteUser(target, role, channel string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}
	if err := a.sdk.PromoteUser(target, role, channel); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) KickUser(target, channel string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}
	if err := a.sdk.KickUser(target, channel); err != nil {
		return err.Error()
	}
	return ""
}

// ExportIdentity opens a save dialog and copies the identity file
func (a *App) ExportIdentity() string {
	if a.sdk == nil {
		return "SDK not initialized"
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "identity_backup.key",
		Title:           "Backup Identity Key",
		Filters: []runtime.FileFilter{
			{DisplayName: "Key Files (*.key)", Pattern: "*.key"},
		},
	})

	if err != nil || path == "" {
		return "Cancelled or Error"
	}

	srcPath := a.sdk.GetIdentityPath()
	source, err := os.Open(srcPath)
	if err != nil {
		return fmt.Sprintf("Failed to open source: %v", err)
	}
	defer source.Close()

	dest, err := os.Create(path)
	if err != nil {
		return fmt.Sprintf("Failed to create destination: %v", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, source); err != nil {
		return fmt.Sprintf("Failed to copy: %v", err)
	}

	return "" // Success
}

func (a *App) GetRole() string {
	// Mock implementation for MVP. Real one would query Channel Service or check local token.
	return "User (Admin Candidate)"
}

func (a *App) GetGridStats() map[string]int {
	if a.sdk == nil {
		return map[string]int{"jobs_processed": 0}
	}
	return a.sdk.GetGridStats()
}

// Channel Hub Methods

func (a *App) SaveWikiPage(channelID, title, content string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}
	if err := a.sdk.SaveWikiPage(channelID, title, content); err != nil {
		return err.Error()
	}
	return "success"
}

func (a *App) GetWikiPages(channelID string) []storage.WikiPage {
	if a.sdk == nil {
		return nil
	}
	pages, _ := a.sdk.GetWikiPages(channelID)
	return pages
}

func (a *App) SetChannelConfig(channelID, key, value string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}
	if err := a.sdk.SetChannelConfig(channelID, key, value); err != nil {
		return err.Error()
	}
	return "success"
}

func (a *App) GetChannelConfig(channelID, key string) string {
	if a.sdk == nil {
		return ""
	}
	val, _ := a.sdk.GetChannelConfig(channelID, key)
	return val
}

func (a *App) UpdateChannelAvatar(channelID, avatar string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}
	if err := a.sdk.UpdateChannelAvatar(channelID, avatar); err != nil {
		return err.Error()
	}
	return ""
}

// ==========================================
// TAGS SYSTEM METHODS
// ==========================================

func (a *App) CreateTag(channelID, name, color string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}
	if err := a.sdk.CreateTag(channelID, name, color); err != nil {
		return err.Error()
	}
	return "success"
}

func (a *App) TagMessage(channelID, messageID, tagID string) string {
	if a.sdk == nil {
		return "SDK not initialized"
	}
	if err := a.sdk.TagMessage(channelID, messageID, tagID); err != nil {
		return err.Error()
	}
	return "success"
}

func (a *App) GetTags(channelID string) []storage.ChannelTag {
	if a.sdk == nil {
		return nil
	}
	tags, _ := a.sdk.GetTags(channelID)
	return tags
}

func (a *App) GetMessageTags(channelID string) map[string][]storage.MessageTag {
	if a.sdk == nil {
		return nil
	}
	tags, _ := a.sdk.GetMessageTags(channelID)
	return tags
}
