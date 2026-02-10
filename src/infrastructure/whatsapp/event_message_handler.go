package whatsapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/chatwoot" // <--- Importante: Importar o pacote chatwoot
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

	// Sincronizar Avatar com Chatwoot (NOVO)
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

func handleWebhookForward(ctx context.Context, evt *events.Message, client *whatsmeow.Client) {
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

// handleChatwootSync aciona a sincronização do avatar/contato quando chega mensagem
func handleChatwootSync(ctx context.Context, evt *events.Message, client *whatsmeow.Client) {
	if !config.ChatwootEnabled {
		return
	}
	// Se a mensagem for minha, não preciso atualizar o contato do destinatário agora
	if evt.Info.IsFromMe {
		return
	}

	// Obtém a instância do serviço de sync (que deve ter sido inicializada no start da aplicação)
	syncSvc := chatwoot.GetDefaultSyncService()
	if syncSvc == nil {
		// Serviço ainda não inicializado, ignorar
		return
	}

	senderJID := evt.Info.Sender.ToNonAD().String()

	// Roda em background para não travar o processamento da mensagem
	go func() {
		// Cria um contexto novo de background, pois o ctx da mensagem pode ser cancelado rápido
		bgCtx := context.Background()
		if err := syncSvc.SyncContactAvatar(bgCtx, senderJID, client); err != nil {
			// Log nível Debug/Warn para não poluir demais caso falhe sempre
			logrus.Debugf("Chatwoot Sync: Failed to auto-sync avatar for %s: %v", senderJID, err)
		}
	}()
}
