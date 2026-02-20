package chatwoot

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"net/http"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	waTypes "go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// SyncService handles message history synchronization to Chatwoot
type SyncService struct {
	client          *Client
	chatStorageRepo domainChatStorage.IChatStorageRepository

	// Track sync progress per device
	progressMap map[string]*SyncProgress
	progressMu  sync.RWMutex
}

// NewSyncService creates a new sync service instance
func NewSyncService(
	client *Client,
	chatStorageRepo domainChatStorage.IChatStorageRepository,
) *SyncService {
	return &SyncService{
		client:          client,
		chatStorageRepo: chatStorageRepo,
		progressMap:     make(map[string]*SyncProgress),
	}
}

// GetProgress returns the current sync progress for a device
func (s *SyncService) GetProgress(deviceID string) *SyncProgress {
	s.progressMu.RLock()
	defer s.progressMu.RUnlock()

	if progress, ok := s.progressMap[deviceID]; ok {
		cloned := progress.Clone()
		return &cloned
	}
	return nil
}

// IsRunning returns true if a sync is currently running for the device
func (s *SyncService) IsRunning(deviceID string) bool {
	s.progressMu.RLock()
	defer s.progressMu.RUnlock()

	if progress, ok := s.progressMap[deviceID]; ok {
		return progress.IsRunning()
	}
	return false
}

func messageKey(deviceID, chatJID string, msg *domainChatStorage.Message) string {
	h := fnv.New64a()
	h.Write([]byte(deviceID))
	h.Write([]byte("|"))
	h.Write([]byte(chatJID))
	h.Write([]byte("|"))
	h.Write([]byte(msg.Timestamp.UTC().Format(time.RFC3339Nano)))
	h.Write([]byte("|"))
	h.Write([]byte(msg.Sender))
	h.Write([]byte("|"))
	h.Write([]byte(msg.Content))
	h.Write([]byte("|"))
	h.Write([]byte(msg.MediaType))
	h.Write([]byte("|"))
	h.Write([]byte(msg.URL))
	return fmt.Sprintf("%x", h.Sum64())
}

// SyncHistory performs the initial message history sync to Chatwoot
func (s *SyncService) SyncHistory(ctx context.Context, deviceID string, waClient *whatsmeow.Client, opts SyncOptions) (*SyncProgress, error) {
	// Atomic check-and-set to prevent race condition
	progress := NewSyncProgress(deviceID)
	s.progressMu.Lock()
	if existing, ok := s.progressMap[deviceID]; ok && existing.IsRunning() {
		s.progressMu.Unlock()
		cloned := existing.Clone()
		return &cloned, fmt.Errorf("sync already in progress for device %s", deviceID)
	}
	s.progressMap[deviceID] = progress
	s.progressMu.Unlock()

	progress.SetRunning()

	logrus.Infof("Chatwoot Sync: Starting history sync for device %s (days: %d, media: %v, groups: %v)",
		deviceID, opts.DaysLimit, opts.IncludeMedia, opts.IncludeGroups)

	// 1. Get all chats for this device
	chats, err := s.chatStorageRepo.GetChats(&domainChatStorage.ChatFilter{
		DeviceID: deviceID,
	})
	if err != nil {
		progress.SetFailed(err)
		return progress, fmt.Errorf("failed to get chats: %w", err)
	}

	progress.SetTotals(len(chats), 0)
	logrus.Infof("Chatwoot Sync: Found %d chats to sync", len(chats))

	// 2. Calculate time boundary
	sinceTime := time.Now().AddDate(0, 0, -opts.DaysLimit)

	// 3. Process each chat
	for _, chat := range chats {
		if err := ctx.Err(); err != nil {
			progress.SetFailed(err)
			return progress, err // Context cancelled
		}

		progress.UpdateChat(chat.JID)

		err := s.syncChat(ctx, deviceID, chat, sinceTime, waClient, opts, progress)
		if err != nil {
			logrus.Errorf("Chatwoot Sync: Failed to sync chat %s: %v", chat.JID, err)
			progress.IncrementFailedChats()
			// Continue with other chats
		} else {
			progress.IncrementSyncedChats()
		}
	}

	progress.SetCompleted()
	logrus.Infof("Chatwoot Sync: Completed for device %s. Chats: %d (failed: %d), Messages: %d (failed: %d)",
		deviceID, progress.SyncedChats, progress.FailedChats, progress.SyncedMessages, progress.FailedMessages)

	return progress, nil
}

// syncChat syncs a single chat's messages to Chatwoot
func (s *SyncService) syncChat(
	ctx context.Context,
	deviceID string,
	chat *domainChatStorage.Chat,
	sinceTime time.Time,
	waClient *whatsmeow.Client,
	opts SyncOptions,
	progress *SyncProgress,
) error {
	isGroup := strings.HasSuffix(chat.JID, "@g.us")
	if isGroup && !opts.IncludeGroups {
		return nil
	}

	contactName := chat.Name
	if contactName == "" {
		contactName = utils.ExtractPhoneFromJID(chat.JID)
	}

	contact, err := s.client.FindOrCreateContact(contactName, chat.JID, isGroup)
	if err != nil {
		return fmt.Errorf("failed to find/create contact: %w", err)
	}

	conversation, err := s.client.FindOrCreateConversation(contact.ID)
	if err != nil {
		return fmt.Errorf("failed to find/create conversation: %w", err)
	}

	state, err := s.chatStorageRepo.GetChatExportState(deviceID, chat.JID)
	if err != nil {
		return fmt.Errorf("failed to get export state: %w", err)
	}

	start := sinceTime
	if state != nil && state.LastExportedAt.After(start) {
		start = state.LastExportedAt
	}

	messages, err := s.chatStorageRepo.GetMessages(&domainChatStorage.MessageFilter{
		DeviceID:  deviceID,
		ChatJID:   chat.JID,
		StartTime: &start,
		Limit:     opts.MaxMessagesPerChat,
	})
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}
	if len(messages) == 0 {
		return nil
	}

	progress.AddMessages(len(messages))

	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})

	var lastExported time.Time
	for i, msg := range messages {
		if err := ctx.Err(); err != nil {
			return err
		}

		key := messageKey(deviceID, chat.JID, msg)

		exported, err := s.chatStorageRepo.IsMessageExported(deviceID, chat.JID, key)
		if err != nil {
			progress.IncrementFailedMessages()
			continue
		}
		if exported {
			continue
		}

		chatwootMsgID, err := s.syncMessageReturnID(ctx, conversation.ID, msg, waClient, opts, isGroup, key)
		if err != nil {
			progress.IncrementFailedMessages()
			continue
		}

		_ = s.chatStorageRepo.MarkMessageExported(deviceID, chat.JID, key, chatwootMsgID)
		lastExported = msg.Timestamp

		if i > 0 && i%opts.BatchSize == 0 {
			time.Sleep(opts.DelayBetweenBatches)
		}
	}

	if !lastExported.IsZero() {
		st := &domainChatStorage.ChatExportState{
			DeviceID:       deviceID,
			ChatJID:        chat.JID,
			LastExportedAt: lastExported,
		}
		_ = s.chatStorageRepo.UpsertChatExportState(st)
	}

	go func() {
		_ = s.SyncContactAvatarSmart(context.Background(), chat.JID, contactName, waClient)
	}()

	return nil
}
func (s *SyncService) syncMessageReturnID(
	ctx context.Context,
	conversationID int,
	msg *domainChatStorage.Message,
	waClient *whatsmeow.Client,
	opts SyncOptions,
	isGroup bool,
	sourceID string,
) (int, error) {
	messageType := "incoming"
	if msg.IsFromMe {
		messageType = "outgoing"
	}

	content := msg.Content
	if content == "" && msg.MediaType != "" {
		content = fmt.Sprintf("[%s]", msg.MediaType)
	}

	timePrefix := msg.Timestamp.Format("2006-01-02 15:04")
	if isGroup && !msg.IsFromMe && msg.Sender != "" {
		senderName := utils.ExtractPhoneFromJID(msg.Sender)
		content = fmt.Sprintf("[%s] %s: %s", timePrefix, senderName, content)
	} else {
		content = fmt.Sprintf("[%s] %s", timePrefix, content)
	}

	var attachments []string
	if opts.IncludeMedia && msg.MediaType != "" && msg.URL != "" && len(msg.MediaKey) > 0 {
		fp, err := s.downloadMedia(ctx, msg, waClient)
		if err == nil && fp != "" {
			attachments = append(attachments, fp)
		} else {
			content += " [media unavailable]"
		}
	}
	chatwootMsgID, err := s.client.CreateMessage(conversationID, content, messageType, attachments, sourceID, "")

	for _, fp := range attachments {
		_ = os.Remove(fp)
	}

	if err != nil {
		return 0, err
	}

	MarkMessageAsSent(chatwootMsgID)
	return chatwootMsgID, nil
}

// downloadMedia downloads media for a message and returns the temp file path
func (s *SyncService) downloadMedia(ctx context.Context, msg *domainChatStorage.Message, waClient *whatsmeow.Client) (string, error) {
	if msg.URL == "" || len(msg.MediaKey) == 0 {
		return "", fmt.Errorf("missing media URL or key")
	}

	if waClient == nil {
		return "", fmt.Errorf("WhatsApp client not available")
	}

	// Create downloadable message based on type
	var downloadable whatsmeow.DownloadableMessage

	switch msg.MediaType {
	case "image":
		downloadable = &waE2E.ImageMessage{
			URL:           proto.String(msg.URL),
			MediaKey:      msg.MediaKey,
			FileSHA256:    msg.FileSHA256,
			FileEncSHA256: msg.FileEncSHA256,
			FileLength:    proto.Uint64(msg.FileLength),
		}
	case "video":
		downloadable = &waE2E.VideoMessage{
			URL:           proto.String(msg.URL),
			MediaKey:      msg.MediaKey,
			FileSHA256:    msg.FileSHA256,
			FileEncSHA256: msg.FileEncSHA256,
			FileLength:    proto.Uint64(msg.FileLength),
		}
	case "audio", "ptt":
		downloadable = &waE2E.AudioMessage{
			URL:           proto.String(msg.URL),
			MediaKey:      msg.MediaKey,
			FileSHA256:    msg.FileSHA256,
			FileEncSHA256: msg.FileEncSHA256,
			FileLength:    proto.Uint64(msg.FileLength),
		}
	case "document":
		downloadable = &waE2E.DocumentMessage{
			URL:           proto.String(msg.URL),
			MediaKey:      msg.MediaKey,
			FileSHA256:    msg.FileSHA256,
			FileEncSHA256: msg.FileEncSHA256,
			FileLength:    proto.Uint64(msg.FileLength),
		}
	case "sticker":
		downloadable = &waE2E.StickerMessage{
			URL:           proto.String(msg.URL),
			MediaKey:      msg.MediaKey,
			FileSHA256:    msg.FileSHA256,
			FileEncSHA256: msg.FileEncSHA256,
			FileLength:    proto.Uint64(msg.FileLength),
		}
	default:
		return "", fmt.Errorf("unsupported media type: %s", msg.MediaType)
	}

	// Download
	data, err := waClient.Download(ctx, downloadable)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	// Write to temp file
	ext := getExtensionForMediaType(msg.MediaType, msg.Filename)
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("chatwoot-sync-*%s", ext))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(data); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write media: %w", err)
	}

	return tmpFile.Name(), nil
}

// Global sync service instance for REST endpoints
var (
	globalSyncService     *SyncService
	globalSyncServiceOnce sync.Once
)

// GetSyncService returns a shared sync service instance
func GetSyncService(
	client *Client,
	chatStorageRepo domainChatStorage.IChatStorageRepository,
) *SyncService {
	globalSyncServiceOnce.Do(func() {
		globalSyncService = NewSyncService(client, chatStorageRepo)
	})
	return globalSyncService
}

// GetDefaultSyncService returns the global sync service if initialized
func GetDefaultSyncService() *SyncService {
	return globalSyncService
}
func (s *SyncService) Reconcile(ctx context.Context, deviceID, chatID string, since time.Time, waClient *whatsmeow.Client) error {
	isGroup := strings.HasSuffix(chatID, "@g.us")
	contactName := utils.ExtractPhoneFromJID(chatID)

	// 1. Acha o contato e a conversa corretamente
	contact, err := s.client.FindOrCreateContact(contactName, chatID, isGroup)
	if err != nil {
		return err
	}

	conversation, err := s.client.FindOrCreateConversation(contact.ID)
	if err != nil {
		return err
	}

	// 2. Pega mensagens do BD (Gowa) formatando o filtro do jeito certo
	waMsgs, err := s.chatStorageRepo.GetMessages(&domainChatStorage.MessageFilter{
		DeviceID:  deviceID,
		ChatJID:   chatID,
		StartTime: &since,
		Limit:     5000,
	})
	if err != nil {
		return err
	}

	want := make(map[string]*domainChatStorage.Message, len(waMsgs))
	for _, m := range waMsgs {
		id := messageKey(deviceID, chatID, m)
		want[id] = m
	}

	// 3. Pega mensagens do Chatwoot usando a função nova
	cwMsgs, err := s.client.GetConversationMessages(conversation.ID)
	if err != nil {
		return err
	}

	existing := make(map[string]int)
	for _, m := range cwMsgs {
		if m.SourceID != "" {
			existing[m.SourceID] = m.ID
		}
	}

	// 4. Deleção do que sumiu no WhatsApp
	for src, msgID := range existing {
		if _, ok := want[src]; !ok {
			_ = s.client.DeleteMessage(conversation.ID, msgID)
			logrus.Infof("Chatwoot Sync: Deleted orphaned message %d", msgID)
		}
	}

	// 5. Criação do que tá faltando no Chatwoot
	for src, waMsg := range want {
		if _, ok := existing[src]; ok {
			continue // Já existe
		}

		messageType := "incoming"
		if waMsg.IsFromMe {
			messageType = "outgoing"
		}

		content := waMsg.Content
		if content == "" && waMsg.MediaType != "" {
			content = fmt.Sprintf("[%s]", waMsg.MediaType)
		}

		timePrefix := waMsg.Timestamp.Format("2006-01-02 15:04")
		if isGroup && !waMsg.IsFromMe && waMsg.Sender != "" {
			senderName := utils.ExtractPhoneFromJID(waMsg.Sender)
			content = fmt.Sprintf("[%s] %s: %s", timePrefix, senderName, content)
		} else {
			content = fmt.Sprintf("[%s] %s", timePrefix, content)
		}

		var attachments []string
		if waMsg.MediaType != "" && waMsg.URL != "" && len(waMsg.MediaKey) > 0 {
			fp, err := s.downloadMedia(ctx, waMsg, waClient)
			if err == nil && fp != "" {
				attachments = append(attachments, fp)
			}
		}

		// Cria a mensagem enviando o sourceID
		_, err := s.client.CreateMessage(conversation.ID, content, messageType, attachments, src, "")
		if err != nil {
			logrus.Errorf("Chatwoot Sync: Failed to create missing message: %v", err)
		}

		for _, fp := range attachments {
			_ = os.Remove(fp)
		}
	}

	return nil
}

// SyncContactAvatar synchronizes the contact's avatar from WhatsApp to Chatwoot
func (s *SyncService) SyncContactAvatar(ctx context.Context, contactJID string, waClient *whatsmeow.Client) error {
	if waClient == nil {
		return fmt.Errorf("whatsapp client is nil")
	}

	// 1. Busca/Cria o contato no Chatwoot para garantir que temos o ID
	// Usamos o JID como nome temporário se não tivermos outro, a função FindOrCreate lida com a busca
	isGroup := strings.HasSuffix(contactJID, "@g.us")
	name := utils.ExtractPhoneFromJID(contactJID) // Ou busque o nome real se tiver disponível
	contact, err := s.client.FindOrCreateContact(name, contactJID, isGroup)
	if err != nil {
		return fmt.Errorf("failed to find/create contact: %w", err)
	}

	// 2. Atualiza o JID (Identifier) se estiver faltando ou diferente
	// Isso garante que o link entre Zap e Chatwoot esteja correto pelo identifier
	if contact.Identifier != contactJID {
		attrs := map[string]interface{}{
			"waha_whatsapp_jid": contactJID,
		}
		if err := s.client.UpdateContactAttributes(contact.ID, contactJID, attrs, isGroup); err != nil {
			logrus.Warnf("Chatwoot Sync: Failed to update contact attributes for %s: %v", contactJID, err)
			// Não retorna erro fatal, tenta atualizar a foto mesmo assim
		} else {
			logrus.Debugf("Chatwoot Sync: Updated JID for contact %d to %s", contact.ID, contactJID)
		}
	}

	// 3. Obtém a URL da foto de perfil do WhatsApp
	jid, _ := waTypes.ParseJID(contactJID)
	picInfo, err := waClient.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{
		Preview: false,
	})

	if err != nil {
		// Se der erro 404 (sem foto) ou outro, apenas logamos e saímos
		logrus.Debugf("Chatwoot Sync: No profile picture found for %s: %v", contactJID, err)
		return nil
	}

	if picInfo == nil || picInfo.URL == "" {
		return nil
	}

	// 4. Baixa a imagem da URL retornada pelo WhatsApp
	resp, err := http.Get(picInfo.URL)
	if err != nil {
		return fmt.Errorf("failed to download profile picture: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download profile picture, status: %d", resp.StatusCode)
	}

	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read profile picture data: %w", err)
	}

	// 5. Envia para o Chatwoot
	if err := s.client.UpdateContactAvatar(contact.ID, imgData); err != nil {
		return fmt.Errorf("failed to update chatwoot avatar: %w", err)
	}

	logrus.Infof("Chatwoot Sync: Profile picture updated for %s", contactJID)
	return nil
}

// TriggerAutoSync is called when a device connects to optionally start auto-sync
func TriggerAutoSync(deviceID string, chatStorageRepo domainChatStorage.IChatStorageRepository, waClient *whatsmeow.Client) {
	if !config.ChatwootEnabled || !config.ChatwootImportMessages {
		return
	}

	client := GetDefaultClient()
	if !client.IsConfigured() {
		logrus.Warn("Chatwoot Sync: Auto-sync skipped - Chatwoot not configured")
		return
	}

	// Resolve the storage device ID (JID) from the WhatsApp client,
	// since chats are stored under the full JID, not the user-assigned alias.
	storageDeviceID := deviceID
	if waClient != nil && waClient.Store != nil && waClient.Store.ID != nil {
		if jid := waClient.Store.ID.ToNonAD().String(); jid != "" {
			storageDeviceID = jid
		}
	}

	syncService := GetSyncService(client, chatStorageRepo)

	go func() {
		opts := DefaultSyncOptions()
		opts.DaysLimit = config.ChatwootDaysLimitImportMessages

		logrus.Infof("Chatwoot Sync: Auto-sync triggered for device %s", storageDeviceID)

		_, err := syncService.SyncHistory(context.Background(), storageDeviceID, waClient, opts)
		if err != nil {
			logrus.Errorf("Chatwoot Sync: Auto-sync failed for device %s: %v", storageDeviceID, err)
		}
	}()
}
