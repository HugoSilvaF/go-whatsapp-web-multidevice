package whatsapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/chatwoot"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func handleMessage(ctx context.Context, evt *events.Message, chatStorageRepo domainChatStorage.IChatStorageRepository, client *whatsmeow.Client) {
	// Log message metadata
	metaParts := buildMessageMetaParts(evt)
	log.Infof("Received message %s from %s (%s): %+v",
		evt.Info.ID,
		evt.Info.SourceString(),
		strings.Join(metaParts, ", "),
		evt.Message,
	)

	if err := chatStorageRepo.CreateMessage(ctx, evt); err != nil {
		// Log storage errors to avoid silent failures that could lead to data loss
		log.Errorf("Failed to store incoming message %s: %v", evt.Info.ID, err)
	}

	// Handle image message if present
	handleImageMessage(ctx, evt, client)

	// Auto-mark message as read if configured
	handleAutoMarkRead(ctx, evt, client)

	// Handle auto-reply if configured
	handleAutoReply(ctx, evt, chatStorageRepo, client)

	// Forward to webhook if configured
	handleWebhookForward(ctx, evt, client)

	// Sync avatar with Chatwoot.
	logrus.Debugf("Chatwoot Sync: Checking if avatar sync is needed for message %s from %s", evt.Info.ID, evt.Info.SourceString())
	handleChatwootSync(ctx, evt, client)
}

func buildMessageMetaParts(evt *events.Message) []string {
	metaParts := []string{
		fmt.Sprintf("pushname: %s", evt.Info.PushName),
		fmt.Sprintf("timestamp: %s", evt.Info.Timestamp),
	}
	if evt.Info.Type != "" {
		metaParts = append(metaParts, fmt.Sprintf("type: %s", evt.Info.Type))
	}
	if evt.Info.Category != "" {
		metaParts = append(metaParts, fmt.Sprintf("category: %s", evt.Info.Category))
	}
	if evt.IsViewOnce {
		metaParts = append(metaParts, "view once")
	}
	return metaParts
}

func handleImageMessage(ctx context.Context, evt *events.Message, client *whatsmeow.Client) {
	if !config.WhatsappAutoDownloadMedia {
		return
	}
	if client == nil {
		return
	}
	if img := evt.Message.GetImageMessage(); img != nil {
		if extracted, err := utils.ExtractMedia(ctx, client, config.PathStorages, img); err != nil {
			log.Errorf("Failed to download image: %v", err)
		} else {
			log.Infof("Image downloaded to %s", extracted.MediaPath)
		}
	}
}

func handleAutoMarkRead(ctx context.Context, evt *events.Message, client *whatsmeow.Client) {
	// Only mark read if auto-mark read is enabled and message is incoming
	if !config.WhatsappAutoMarkRead || evt.Info.IsFromMe {
		return
	}

	if client == nil {
		return
	}

	// Mark the message as read
	messageIDs := []types.MessageID{evt.Info.ID}
	timestamp := time.Now()
	chat := evt.Info.Chat
	sender := evt.Info.Sender

	if err := client.MarkRead(ctx, messageIDs, timestamp, chat, sender); err != nil {
		log.Warnf("Failed to mark message %s as read: %v", evt.Info.ID, err)
	} else {
		log.Debugf("Marked message %s as read", evt.Info.ID)
	}
}

func handleWebhookForward(_ctx context.Context, evt *events.Message, client *whatsmeow.Client) {
	// Skip webhook for protocol messages that are internal sync messages
	if protocolMessage := evt.Message.GetProtocolMessage(); protocolMessage != nil {
		protocolType := protocolMessage.GetType().String()
		switch protocolType {
		case "REVOKE", "MESSAGE_EDIT":
			// These are meaningful user actions, allow webhook
		default:
			log.Debugf("Skipping webhook for protocol message type: %s", protocolType)
			return
		}
	}

	if (len(config.WhatsappWebhook) > 0 || config.ChatwootEnabled) &&
		!strings.Contains(evt.Info.SourceString(), "broadcast") {
		go func(e *events.Message, c *whatsmeow.Client) {
			webhookCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := forwardMessageToWebhook(webhookCtx, c, e); err != nil {
				logrus.Error("Failed forward to webhook: ", err)
			}
		}(evt, client)
	}
}

func handleChatwootSync(ctx context.Context, evt *events.Message, client *whatsmeow.Client) {
	logrus.Debugf("Chatwoot Sync: Avatar sync is enabled, processing message %s", evt.Info.ID)
	if !config.ChatwootEnabled {
		logrus.Debugf("Chatwoot Sync: Chatwoot integration is not enabled, skipping avatar sync for message %s", evt.Info.ID)
		return
	}

	if client == nil {
		logrus.Debugf("Chatwoot Sync: WhatsApp client is nil, skipping avatar sync for message %s", evt.Info.ID)
		return
	}

	// If the message is from me, skip avatar sync for this event.
	if evt.Info.IsFromMe {
		logrus.Debugf("Chatwoot Sync: Message %s is from me, skipping avatar sync", evt.Info.ID)
		return
	}

	logrus.Debugf("Chatwoot Sync: Attempting to sync avatar for message %s from %s", evt.Info.ID, evt.Info.SourceString())

	realJID := NormalizeJIDFromLID(ctx, evt.Info.Sender.ToNonAD(), client)
	senderJID := realJID.String()

	// Run in background to avoid blocking message processing.
	go func() {
		logrus.Debugf("Chatwoot Sync: Attempting to auto-sync avatar for %s", senderJID)
		syncSvc := chatwoot.GetDefaultSyncService()
		if syncSvc == nil {
			logrus.Debugf("Chatwoot Sync: Sync service is not initialized, skipping avatar sync for %s", senderJID)
			return
		}

		syncCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := syncSvc.SyncContactAvatarSmart(syncCtx, senderJID, evt.Info.PushName, client); err != nil {
			logrus.Debugf("Chatwoot Sync: Failed avatar sync for %s: %v", senderJID, err)
		}

		logrus.Debugf("Chatwoot Sync: Finished avatar sync for %s", senderJID)
	}()
}
