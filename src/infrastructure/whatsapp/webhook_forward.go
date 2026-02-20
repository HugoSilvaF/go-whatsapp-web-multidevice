package whatsapp

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/chatwoot"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow/types"
)

var submitWebhookFn = submitWebhook

const mutexShardCount = 64

var contactMutexShards [mutexShardCount]sync.Mutex

type groupNameCacheEntry struct {
	name      string
	expiresAt time.Time
}

var (
	groupNameCache    sync.Map
	groupNameCacheTTL = 5 * time.Minute

	chatwootForwardDeduper = struct {
		mu   sync.Mutex
		seen map[string]time.Time
	}{
		seen: make(map[string]time.Time),
	}
	chatwootForwardDeduperTTL = 2 * time.Minute
)

func getCachedGroupName(groupJID string) (string, bool) {
	if entry, ok := groupNameCache.Load(groupJID); ok {
		cached := entry.(groupNameCacheEntry)
		if time.Now().Before(cached.expiresAt) {
			return cached.name, true
		}
		groupNameCache.Delete(groupJID)
	}
	return "", false
}

func setCachedGroupName(groupJID, name string) {
	groupNameCache.Store(groupJID, groupNameCacheEntry{
		name:      name,
		expiresAt: time.Now().Add(groupNameCacheTTL),
	})
}

func getContactMutex(phone string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(phone))
	return &contactMutexShards[h.Sum32()%mutexShardCount]
}

func forwardPayloadToConfiguredWebhooks(ctx context.Context, payload map[string]any, eventName string) error {
	if len(config.WhatsappWebhookEvents) > 0 {
		if !isEventWhitelisted(eventName) {
			logrus.Debugf("Skipping event %s - not in webhook events whitelist", eventName)
			return nil
		}
	}

	err := forwardToWebhooks(ctx, payload, eventName)

	if eventName == "message" && config.ChatwootEnabled {
		go forwardToChatwoot(ctx, payload)
	}

	return err
}

func forwardToWebhooks(ctx context.Context, payload map[string]any, eventName string) error {
	total := len(config.WhatsappWebhook)
	logrus.Infof("Forwarding %s to %d configured webhook(s)", eventName, total)

	if total == 0 {
		return nil
	}

	var (
		failed    []string
		successes int
	)
	for _, url := range config.WhatsappWebhook {
		if err := submitWebhookFn(ctx, payload, url); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", url, err))
			logrus.Warnf("Failed forwarding %s to %s: %v", eventName, url, err)
			continue
		}
		successes++
	}

	if len(failed) > 0 {
		logrus.Warnf("Some webhook URLs failed for %s (succeeded: %d/%d): %s", eventName, successes, total, strings.Join(failed, "; "))
		if successes == 0 {
			return fmt.Errorf("all %d webhook(s) failed for %s", total, eventName)
		}
	} else {
		logrus.Infof("%s forwarded to all webhook(s)", eventName)
	}

	return nil
}

type chatwootContactInfo struct {
	Identifier string
	Name       string
	IsGroup    bool
	FromName   string
	IsFromMe   bool
}

func extractChatwootContactInfo(ctx context.Context, data map[string]interface{}) (*chatwootContactInfo, error) {
	from, _ := data["from"].(string)
	fromName, _ := data["from_name"].(string)
	chatID, _ := data["chat_id"].(string)
	isFromMe, _ := data["is_from_me"].(bool)

	logrus.Infof("Chatwoot: Processing message from %s (from_name: %s, chat_id: %s, is_from_me: %v)", from, fromName, chatID, isFromMe)

	if from == "" {
		return nil, fmt.Errorf("empty 'from' field")
	}

	isGroup := utils.IsGroupJID(chatID)
	info := &chatwootContactInfo{
		IsGroup:  isGroup,
		FromName: fromName,
		IsFromMe: isFromMe,
	}

	if isGroup {
		info.Identifier = chatID
		info.Name = getGroupName(ctx, chatID)
		if info.Name == "" {
			info.Name = "Group: " + utils.ExtractPhoneFromJID(chatID)
		}
		logrus.Infof("Chatwoot: Detected group message, using group contact: %s", info.Name)
	} else if isFromMe {
		info.Identifier = utils.ExtractPhoneFromJID(chatID)
		info.Name = info.Identifier
	} else {
		info.Identifier = utils.ExtractPhoneFromJID(from)
		info.Name = fromName
		if info.Name == "" {
			info.Name = info.Identifier
		}
	}

	return info, nil
}

func classifyMessageSupport(data map[string]interface{}, content string, attachments []string) (bool, string) {
	if content != "" || len(attachments) > 0 {
		return true, ""
	}

	if t, ok := data["type"].(string); ok {
		switch t {
		case "sticker":
			return true, "(Sticker)"
		case "ephemeral":
			return false, ""
		case "protocol":
			return false, ""
		default:
			return true, fmt.Sprintf("(Unsupported: %s)", t)
		}
	}

	return false, ""
}

var mediaFields = []string{"image", "audio", "video", "document", "sticker", "video_note"}

func buildChatwootMessageContent(data map[string]interface{}, isGroup bool, fromName string) (string, []string, bool) {
	content := extractBaseContent(data)
	content, isEdited := extractEditedContent(data, content)
	attachments := extractAttachments(data)

	supported, fallback := classifyMessageSupport(data, content, attachments)
	if !supported {
		return "", nil, false
	}

	if content == "" && fallback != "" {
		content = fallback
	}

	if isEdited && content != "" {
		content = "✏️ Editado: " + content
	}

	if isGroup && fromName != "" {
		if content != "" {
			content = fromName + ": " + content
		} else if len(attachments) > 0 {
			content = fromName + ": (media)"
		}
	}

	return content, attachments, true
}

func extractBaseContent(data map[string]interface{}) string {
	if body, ok := data["body"].(string); ok && body != "" {
		return body
	}
	return extractStructuredMessageContent(data)
}

func extractEditedContent(data map[string]interface{}, content string) (string, bool) {
	if editedMsg, ok := data["edited_msg"].(map[string]interface{}); ok {
		if newBody, ok := editedMsg["body"].(string); ok && newBody != "" {
			return newBody, true
		}
		if newCaption, ok := editedMsg["caption"].(string); ok && newCaption != "" {
			return newCaption, true
		}
	}

	if typeVal, ok := data["type"].(string); ok {
		if typeVal == "edited_msg" || typeVal == "protocol_message" {
			if content != "" {
				return content, true
			}
		}
	}

	return content, false
}

func extractAttachments(data map[string]interface{}) []string {
	attachments := make([]string, 0, len(mediaFields))

	for _, field := range mediaFields {
		mediaData, ok := data[field]
		if !ok {
			continue
		}

		if path, ok := mediaData.(string); ok && path != "" {
			attachments = append(attachments, path)
			continue
		}

		if mediaMap, ok := mediaData.(map[string]interface{}); ok {
			if url, ok := mediaMap["url"].(string); ok && url != "" {
				attachments = append(attachments, url)
			}
		}
	}

	return attachments
}

var skipKeys = []string{
	"reaction",
	"poll_update",
}

var skipMessageTypes = map[string]struct{}{
	"protocol":                {},
	"chat_state":              {},
	"sender_key_distribution": {},
	"revoked":                 {},
	"keep_in_chat":            {},
}

func shouldSkipMessage(data map[string]interface{}) bool {
	for _, key := range skipKeys {
		if _, ok := data[key]; ok {
			return true
		}
	}

	if typeVal, ok := data["type"].(string); ok {
		_, skip := skipMessageTypes[typeVal]
		return skip
	}

	return false
}

func chatwootMessageTypeFromPayload(data map[string]interface{}) string {
	if isFromMe, ok := data["is_from_me"].(bool); ok && isFromMe {
		return "outgoing"
	}
	return "incoming"
}

func extractStructuredMessageContent(data map[string]interface{}) string {
	if contact, ok := data["contact"]; ok && contact != nil {
		if cm, ok := contact.(interface {
			GetDisplayName() string
			GetVcard() string
		}); ok {
			name := cm.GetDisplayName()
			phone := extractPhoneFromVCard(cm.GetVcard())
			switch {
			case name != "" && phone != "":
				return fmt.Sprintf("Contact: %s (%s)", name, phone)
			case name != "":
				return "Contact: " + name
			case phone != "":
				return "Contact: " + phone
			}
		}
		return "Contact shared"
	}

	if location, ok := data["location"]; ok && location != nil {
		if lm, ok := location.(interface {
			GetDegreesLatitude() float64
			GetDegreesLongitude() float64
			GetName() string
		}); ok {
			name := lm.GetName()
			if name != "" {
				return fmt.Sprintf("Location: %s (%.6f, %.6f)", name, lm.GetDegreesLatitude(), lm.GetDegreesLongitude())
			}
			return fmt.Sprintf("Location: %.6f, %.6f", lm.GetDegreesLatitude(), lm.GetDegreesLongitude())
		}
		return "Location shared"
	}

	if liveLocation, ok := data["live_location"]; ok && liveLocation != nil {
		if lm, ok := liveLocation.(interface {
			GetDegreesLatitude() float64
			GetDegreesLongitude() float64
		}); ok {
			return fmt.Sprintf("Live Location: %.6f, %.6f", lm.GetDegreesLatitude(), lm.GetDegreesLongitude())
		}
		return "Live location shared"
	}

	if list, ok := data["list"]; ok && list != nil {
		if lm, ok := list.(interface{ GetTitle() string }); ok {
			title := lm.GetTitle()
			if title != "" {
				return "List: " + title
			}
		}
		return "List message"
	}

	if order, ok := data["order"]; ok && order != nil {
		if om, ok := order.(interface{ GetOrderTitle() string }); ok {
			title := om.GetOrderTitle()
			if title != "" {
				return "Order: " + title
			}
		}
		return "Order message"
	}

	return ""
}

func extractPhoneFromVCard(vcard string) string {
	for _, line := range strings.Split(vcard, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "TEL") {
			if idx := strings.LastIndex(line, ":"); idx >= 0 {
				return strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return ""
}

func syncMessageToChatwoot(cw *chatwoot.Client, info *chatwootContactInfo, content string, attachments []string) error {
	mu := getContactMutex(info.Identifier)
	mu.Lock()

	contact, err := cw.FindOrCreateContact(info.Name, info.Identifier, info.IsGroup)
	if err != nil {
		mu.Unlock()
		return fmt.Errorf("failed to find/create contact for %s: %w", info.Identifier, err)
	}
	logrus.Infof("Chatwoot: Contact ID: %d", contact.ID)

	conversation, err := cw.FindOrCreateConversation(contact.ID)
	mu.Unlock()
	if err != nil {
		return fmt.Errorf("failed to find/create conversation for contact %d: %w", contact.ID, err)
	}
	logrus.Infof("Chatwoot: Conversation ID: %d", conversation.ID)

	logrus.Infof("Chatwoot: Creating message (Length: %d, Attachments: %d)", len(content), len(attachments))
	messageType := "incoming"
	if info.IsFromMe {
		messageType = "outgoing"
	}

	msgID, err := cw.CreateMessage(conversation.ID, content, messageType, attachments, info.Identifier)
	if err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}
	chatwoot.MarkMessageAsSent(msgID)

	logrus.Infof("Chatwoot: Message synced successfully for %s", info.Identifier)
	return nil
}

func forwardToChatwoot(ctx context.Context, payload map[string]any) {
	logrus.Info("Chatwoot: Attempting to forward message...")
	cw := chatwoot.GetDefaultClient()
	if !cw.IsConfigured() {
		logrus.Warn("Chatwoot: Client is not configured (check CHATWOOT_* env vars)")
		return
	}

	data, ok := payload["payload"].(map[string]interface{})
	if !ok {
		logrus.Error("Chatwoot: Invalid payload format (missing 'payload' object)")
		return
	}

	if msgID, _ := data["id"].(string); msgID != "" {
		if isDuplicateChatwootForward(msgID) {
			logrus.Debugf("Chatwoot: Skipping duplicate forward for WhatsApp message %s", msgID)
			return
		}
	}

	if shouldSkipMessage(data) {
		logrus.Debug("Chatwoot: Skipping message type (reaction/poll_update/etc) to prevent spam")
		return
	}

	info, err := extractChatwootContactInfo(ctx, data)
	if err != nil {
		logrus.Warnf("Chatwoot: Skipping message: %v", err)
		return
	}

	content, attachments, supported := buildChatwootMessageContent(data, info.IsGroup, info.FromName)
	if !supported {
		logrus.Debug("Chatwoot: Message classified as not supported for human display")
		return
	}

	if err := syncMessageToChatwoot(cw, info, content, attachments); err != nil {
		logrus.Errorf("Chatwoot: %v", err)
	}
}

func isDuplicateChatwootForward(messageID string) bool {
	now := time.Now()

	chatwootForwardDeduper.mu.Lock()
	defer chatwootForwardDeduper.mu.Unlock()

	for id, ts := range chatwootForwardDeduper.seen {
		if now.Sub(ts) > chatwootForwardDeduperTTL {
			delete(chatwootForwardDeduper.seen, id)
		}
	}

	if ts, exists := chatwootForwardDeduper.seen[messageID]; exists {
		if now.Sub(ts) <= chatwootForwardDeduperTTL {
			return true
		}
	}

	chatwootForwardDeduper.seen[messageID] = now
	return false
}

func isEventWhitelisted(eventName string) bool {
	for _, allowed := range config.WhatsappWebhookEvents {
		if strings.EqualFold(strings.TrimSpace(allowed), eventName) {
			return true
		}
	}
	return false
}

func getGroupName(ctx context.Context, groupJID string) string {
	if name, ok := getCachedGroupName(groupJID); ok {
		logrus.Debugf("Chatwoot: Using cached group name for %s: %s", groupJID, name)
		return name
	}

	client := ClientFromContext(ctx)
	if client == nil {
		logrus.Debug("Chatwoot: ClientFromContext returned nil, trying GetClient()")
		client = GetClient()
	}
	if client == nil {
		logrus.Warn("Chatwoot: No WhatsApp client available to fetch group name")
		return ""
	}

	jid, err := types.ParseJID(groupJID)
	if err != nil {
		logrus.Warnf("Chatwoot: Failed to parse group JID %s: %v", groupJID, err)
		return ""
	}

	freshCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logrus.Debugf("Chatwoot: Fetching group info for %s", groupJID)
	groupInfo, err := client.GetGroupInfo(freshCtx, jid)
	if err != nil {
		logrus.Warnf("Chatwoot: Failed to get group info for %s: %v", groupJID, err)
		return ""
	}

	if groupInfo != nil && groupInfo.Name != "" {
		logrus.Infof("Chatwoot: Got group name: %s", groupInfo.Name)
		setCachedGroupName(groupJID, groupInfo.Name)
		return groupInfo.Name
	}

	logrus.Debug("Chatwoot: GroupInfo is nil or Name is empty")
	return ""
}
