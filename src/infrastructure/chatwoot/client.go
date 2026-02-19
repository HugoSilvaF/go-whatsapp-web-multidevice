package chatwoot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/sirupsen/logrus"
)

type Client struct {
	BaseURL    string
	APIToken   string
	AccountID  int
	InboxID    int
	HTTPClient *http.Client
}

var (
	defaultClient     *Client
	defaultClientOnce sync.Once

	sentMessageIDs    sync.Map
	sentMessageIDsTTL = 5 * time.Minute
)

type shardLocks struct {
	shards []chan struct{}
}

func newShardLocks(n int) *shardLocks {
	s := &shardLocks{shards: make([]chan struct{}, n)}
	for i := 0; i < n; i++ {
		s.shards[i] = make(chan struct{}, 1)
		s.shards[i] <- struct{}{}
	}
	return s
}


func (l *shardLocks) lock(key string) func() {
	idx := int(fnv32a(key) % uint32(len(l.shards)))
	ch := l.shards[idx]
	<-ch
	return func() { ch <- struct{}{} }
}

func GetDefaultClient() *Client {
	defaultClientOnce.Do(func() {
		defaultClient = NewClient()
	})
	return defaultClient
}

func MarkMessageAsSent(messageID int) {
	if messageID == 0 {
		return
	}
	sentMessageIDs.Store(messageID, time.Now())
}

func IsMessageSentByUs(messageID int) bool {
	if messageID == 0 {
		return false
	}
	val, ok := sentMessageIDs.Load(messageID)
	if !ok {
		return false
	}
	storedAt := val.(time.Time)
	if time.Since(storedAt) > sentMessageIDsTTL {
		sentMessageIDs.Delete(messageID)
		return false
	}
	return true
}

func init() {
	go func() {
		ticker := time.NewTicker(sentMessageIDsTTL)
		defer ticker.Stop()
		for range ticker.C {
			sentMessageIDs.Range(func(key, value interface{}) bool {
				if time.Since(value.(time.Time)) > sentMessageIDsTTL {
					sentMessageIDs.Delete(key)
				}
				return true
			})
		}
	}()
}

func NewClient() *Client {
	return &Client{
		BaseURL:   strings.TrimRight(config.ChatwootURL, "/"),
		APIToken: config.ChatwootAPIToken,
		AccountID: config.ChatwootAccountID,
		InboxID:   config.ChatwootInboxID,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) IsConfigured() bool {
	return c.BaseURL != "" && c.APIToken != "" && c.AccountID != 0 && c.InboxID != 0
}

func (c *Client) doRequest(method, endpoint string, payload interface{}, result interface{}) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewBuffer(jsonPayload)
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, err
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bodyBytes, fmt.Errorf("request failed: status %d body %s", resp.StatusCode, string(bodyBytes))
	}

	if result != nil && len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, result); err != nil {
			return bodyBytes, fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return bodyBytes, nil
}

func (c *Client) FindContactByIdentifier(identifier string, isGroup bool) (*Contact, error) {
	endpoint := fmt.Sprintf("%s/api/v1/accounts/%d/contacts/search", c.BaseURL, c.AccountID)
	logrus.Debugf("Chatwoot: Finding contact by identifier endpoint=%s identifier=%s isGroup=%v", endpoint, identifier, isGroup)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	searchTerm := identifier
	isIdentifierBased := isGroup || strings.HasSuffix(identifier, "@lid")
	if !isIdentifierBased {
		searchTerm = utils.NormalizePhoneE164(identifier)
	}

	q := req.URL.Query()
	q.Add("q", searchTerm)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to search contact: status %d body %s", resp.StatusCode, string(body))
	}

	var result struct {
		Payload []Contact `json:"payload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	for _, contact := range result.Payload {
		if isIdentifierBased {
			if contact.Identifier == identifier {
				return &contact, nil
			}
			if contact.CustomAttributes != nil {
				if jid, ok := contact.CustomAttributes["waha_whatsapp_jid"].(string); ok && jid == identifier {
					return &contact, nil
				}
			}
			continue
		}

		if contact.PhoneNumber == searchTerm {
			return &contact, nil
		}
		if contact.CustomAttributes != nil {
			if jid, ok := contact.CustomAttributes["waha_whatsapp_jid"].(string); ok && jid == identifier {
				return &contact, nil
			}
		}
	}

	return nil, nil
}

func (c *Client) CreateContact(name, identifier string, isGroup bool) (*Contact, error) {
	endpoint := fmt.Sprintf("%s/api/v1/accounts/%d/contacts", c.BaseURL, c.AccountID)

	var phoneNumber, contactIdentifier string
	isIdentifierBased := isGroup || strings.HasSuffix(identifier, "@lid")
	if isIdentifierBased {
		contactIdentifier = identifier
	} else {
		phoneNumber = utils.NormalizePhoneE164(identifier)
	}

	payload := CreateContactRequest{
		InboxID:     c.InboxID,
		Name:        name,
		PhoneNumber: phoneNumber,
		Identifier:  contactIdentifier,
		CustomAttributes: map[string]interface{}{
			"waha_whatsapp_jid": identifier,
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal contact payload: %w", err)
	}

	logrus.Debugf("Chatwoot CreateContact: Sending payload: %s", string(jsonPayload))

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	logrus.Debugf("Chatwoot CreateContact: Response status=%d body=%s", resp.StatusCode, string(bodyBytes))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		existing, findErr := c.FindContactByIdentifier(identifier, isGroup)
		if findErr == nil && existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("failed to create contact: status %d body %s", resp.StatusCode, string(bodyBytes))
	}

	var nestedResult struct {
		Payload struct {
			Contact Contact `json:"contact"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(bodyBytes, &nestedResult); err == nil && nestedResult.Payload.Contact.ID != 0 {
		return &nestedResult.Payload.Contact, nil
	}

	var flatResult struct {
		Payload Contact `json:"payload"`
	}
	if err := json.Unmarshal(bodyBytes, &flatResult); err == nil && flatResult.Payload.ID != 0 {
		return &flatResult.Payload, nil
	}

	var contact Contact
	if err := json.Unmarshal(bodyBytes, &contact); err == nil && contact.ID != 0 {
		return &contact, nil
	}

	return nil, fmt.Errorf("failed to decode contact response (no valid ID found): %s", string(bodyBytes))
}

func (c *Client) FindOrCreateContact(name, identifier string, isGroup bool) (*Contact, error) {
	unlock := contactLocks.lock(identifier)
	defer unlock()

	contact, err := c.FindContactByIdentifier(identifier, isGroup)
	if err != nil {
		return nil, err
	}

	if contact != nil {
		if contact.Name != name && name != "" {
			logrus.Infof("Chatwoot: Updating contact name from '%s' to '%s'", contact.Name, name)
			if err := c.UpdateContactName(contact.ID, name); err != nil {
				logrus.Warnf("Chatwoot: Failed to update contact name: %v", err)
			} else {
				contact.Name = name
			}
		}

		if contact.CustomAttributes == nil {
			contact.CustomAttributes = map[string]interface{}{}
		}
		if _, ok := contact.CustomAttributes["waha_whatsapp_jid"].(string); !ok {
			attrs := map[string]interface{}{
				"waha_whatsapp_jid": identifier,
			}
			_ = c.UpdateContactAttributes(contact.ID, identifier, attrs, isGroup)
		}

		return contact, nil
	}

	created, err := c.CreateContact(name, identifier, isGroup)
	if err != nil {
		again, findErr := c.FindContactByIdentifier(identifier, isGroup)
		if findErr == nil && again != nil {
			return again, nil
		}
		return nil, err
	}
	return created, nil
}

func (c *Client) UpdateContactName(contactID int, name string) error {
	endpoint := fmt.Sprintf("%s/api/v1/accounts/%d/contacts/%d", c.BaseURL, c.AccountID, contactID)

	payload := map[string]interface{}{
		"name": name,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal update payload: %w", err)
	}

	req, err := http.NewRequest("PUT", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update contact: status %d body %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) UploadAvatar(contactID int, imageBytes []byte) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("avatar", "avatar.png")
	if err != nil {
		return err
	}
	_, _ = part.Write(imageBytes)
	_ = writer.Close()

	url := fmt.Sprintf("%s/api/v1/accounts/%d/contacts/%d/avatar", c.BaseURL, c.AccountID, contactID)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("falha no upload: status %d body %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *Client) CreateConversation(contactID int) (*Conversation, error) {
	endpoint := fmt.Sprintf("%s/api/v1/accounts/%d/conversations", c.BaseURL, c.AccountID)

	payload := CreateConversationRequest{
		InboxID:   c.InboxID,
		ContactID: contactID,
		Status:    "open",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal conversation payload: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create conversation: status %d body %s", resp.StatusCode, string(bodyBytes))
	}

	logrus.Debugf("Chatwoot CreateConversation: Response body=%s", string(bodyBytes))

	var result struct {
		Payload Conversation `json:"payload"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err == nil && result.Payload.ID != 0 {
		return &result.Payload, nil
	}

	var conversation Conversation
	if err := json.Unmarshal(bodyBytes, &conversation); err == nil && conversation.ID != 0 {
		return &conversation, nil
	}

	return nil, fmt.Errorf("failed to decode conversation response (no valid ID found): %s", string(bodyBytes))
}

func (c *Client) FindConversation(contactID int) (*Conversation, error) {
	endpoint := fmt.Sprintf("%s/api/v1/accounts/%d/contacts/%d/conversations", c.BaseURL, c.AccountID, contactID)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list contact conversations: status %d body %s", resp.StatusCode, string(body))
	}

	var result struct {
		Payload []struct {
			ID      int    `json:"id"`
			InboxID int    `json:"inbox_id"`
			Status  string `json:"status"`
		} `json:"payload"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	for _, conv := range result.Payload {
		if conv.InboxID == c.InboxID && conv.Status != "resolved" {
			return &Conversation{
				ID:        conv.ID,
				ContactID: contactID,
				InboxID:   conv.InboxID,
				Status:    conv.Status,
			}, nil
		}
	}

	return nil, nil
}

func (c *Client) FindOrCreateConversation(contactID int) (*Conversation, error) {
	conv, err := c.FindConversation(contactID)
	if err != nil {
		logrus.Errorf("Error finding conversation: %v", err)
	}
	if conv != nil {
		return conv, nil
	}
	return c.CreateConversation(contactID)
}

func (c *Client) CreateMessage(conversationID int, content string, messageType string, attachments []string) (int, error) {
	endpoint := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages", c.BaseURL, c.AccountID, conversationID)

	if len(attachments) > 0 {
		return c.createMessageWithAttachments(endpoint, content, messageType, attachments)
	}

	payload := CreateMessageRequest{
		Content:     content,
		MessageType: messageType,
		Private:     false,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal message payload: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to create message: status %d body %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err == nil && result.ID != 0 {
		return result.ID, nil
	}

	return 0, nil
}

func (c *Client) createMessageWithAttachments(endpoint, content, messageType string, attachments []string) (int, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	_ = writer.WriteField("content", content)
	_ = writer.WriteField("message_type", messageType)
	_ = writer.WriteField("private", "false")

	recordedAudioFilenames := make([]string, 0, len(attachments))
	recordedAudioSeen := make(map[string]struct{}, len(attachments))

	for _, filePath := range attachments {
		func(fp string) {
			uploadPath, cleanup := prepareAttachmentForUpload(fp)
			defer cleanup()

			file, err := os.Open(uploadPath)
			if err != nil {
				logrus.Errorf("Failed to open file %s: %v", uploadPath, err)
				return
			}
			defer file.Close()

			fileName := filepath.Base(uploadPath)

			rawMimeType := mime.TypeByExtension(filepath.Ext(uploadPath))
			if rawMimeType == "" {
				detectedType, err := detectContentType(uploadPath)
				if err == nil && detectedType != "" {
					rawMimeType = detectedType
				}
			}
			mimeType := normalizeAttachmentMimeType(uploadPath, rawMimeType)
			if mimeType == "" {
				mimeType = canonicalizeMimeType(rawMimeType)
			}
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}

			if shouldMarkAsRecordedAudio(uploadPath, mimeType) {
				if _, seen := recordedAudioSeen[fileName]; !seen {
					recordedAudioSeen[fileName] = struct{}{}
					recordedAudioFilenames = append(recordedAudioFilenames, fileName)
				}
			}

			logrus.Debugf("Chatwoot: attachment prepared filename=%s mime=%s path=%s", fileName, mimeType, uploadPath)

			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="attachments[]"; filename="%s"`, fileName))
			h.Set("Content-Type", mimeType)

			part, err := writer.CreatePart(h)
			if err != nil {
				logrus.Errorf("Failed to create form part for %s: %v", uploadPath, err)
				return
			}
			if _, err := io.Copy(part, file); err != nil {
				logrus.Errorf("Failed to copy file %s to multipart body: %v", uploadPath, err)
				return
			}
		}(filePath)
	}

	if len(recordedAudioFilenames) > 0 {
		logrus.Debugf("Chatwoot: marking audio attachments as recorded audio: %v", recordedAudioFilenames)
		raw, err := json.Marshal(recordedAudioFilenames)
		if err != nil {
			logrus.Warnf("Chatwoot: failed to encode is_recorded_audio metadata: %v", err)
		} else if err := writer.WriteField("is_recorded_audio", string(raw)); err != nil {
			logrus.Warnf("Chatwoot: failed to write is_recorded_audio field: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		return 0, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to create message with attachments: status %d body %s", resp.StatusCode, string(respBody))
	}

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		bodyForLog := string(respBody)
		if len(bodyForLog) > 2048 {
			bodyForLog = bodyForLog[:2048] + "...(truncated)"
		}
		logrus.Debugf("Chatwoot: createMessageWithAttachments response status=%d body=%s", resp.StatusCode, bodyForLog)
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && result.ID != 0 {
		return result.ID, nil
	}

	return 0, nil
}

func (c *Client) UpdateContactAvatar(contactID int, avatarData []byte) error {
	endpoint := fmt.Sprintf("%s/api/v1/accounts/%d/contacts/%d", c.BaseURL, c.AccountID, contactID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	contentType := http.DetectContentType(avatarData)

	ext := ".jpg"
	if contentType == "image/png" {
		ext = ".png"
	}

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="avatar"; filename="profile%s"`, ext))
	h.Set("Content-Type", contentType)

	part, err := writer.CreatePart(h)
	if err != nil {
		return fmt.Errorf("failed to create form part: %w", err)
	}

	if _, err := io.Copy(part, bytes.NewReader(avatarData)); err != nil {
		return fmt.Errorf("failed to write avatar data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	req, err := http.NewRequest("PATCH", endpoint, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update avatar: status %d body %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *Client) UpdateContactAttributes(contactID int, identifier string, customAttributes map[string]interface{}, isGroup bool) error {
	endpoint := fmt.Sprintf("%s/api/v1/accounts/%d/contacts/%d", c.BaseURL, c.AccountID, contactID)

	payload := map[string]interface{}{}

	if identifier != "" && (isGroup || strings.HasSuffix(identifier, "@lid")) {
		payload["identifier"] = identifier
	}

	if customAttributes != nil {
		payload["custom_attributes"] = customAttributes
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal update payload: %w", err)
	}

	req, err := http.NewRequest("PUT", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update contact attributes: status %d body %s", resp.StatusCode, string(body))
	}

	return nil
}
