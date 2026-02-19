package chatwoot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
	waTypes "go.mau.fi/whatsmeow/types"
)

type jidLocks struct {
	shards []chan struct{}
}

func newJIDLocks(n int) *jidLocks {
	ls := &jidLocks{shards: make([]chan struct{}, n)}
	for i := 0; i < n; i++ {
		ls.shards[i] = make(chan struct{}, 1)
		ls.shards[i] <- struct{}{}
	}
	return ls
}

func fnv32a(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func (l *jidLocks) lock(key string) func() {
	idx := int(fnv32a(key) % uint32(len(l.shards)))
	ch := l.shards[idx]
	<-ch
	return func() { ch <- struct{}{} }
}

var contactLocks = newJIDLocks(64)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *SyncService) SyncContactAvatarSmart(
	ctx context.Context,
	contactJID string,
	contactName string,
	waClient *whatsmeow.Client,
) error {
	if waClient == nil {
		return fmt.Errorf("whatsapp client is nil")
	}

	unlock := contactLocks.lock(contactJID)
	defer unlock()

	isGroup := strings.HasSuffix(contactJID, "@g.us")
	if contactName == "" {
		contactName = utils.ExtractPhoneFromJID(contactJID)
	}

	contact, err := s.client.FindOrCreateContact(contactName, contactJID, isGroup)
	if err != nil {
		return err
	}

	jid, err := waTypes.ParseJID(contactJID)
	if err != nil {
		return err
	}

	picInfo, err := waClient.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: false})
	if err != nil || picInfo == nil || picInfo.URL == "" {
		attrs := map[string]interface{}{
			"waha_whatsapp_jid":      contactJID,
			"waha_avatar_checked_at": time.Now().UTC().Format(time.RFC3339),
		}
		_ = s.client.UpdateContactAttributes(contact.ID, contactJID, attrs, isGroup)
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", picInfo.URL, nil)
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: 20 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		attrs := map[string]interface{}{
			"waha_whatsapp_jid":      contactJID,
			"waha_avatar_checked_at": time.Now().UTC().Format(time.RFC3339),
		}
		_ = s.client.UpdateContactAttributes(contact.ID, contactJID, attrs, isGroup)
		return nil
	}

	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(imgData) == 0 {
		attrs := map[string]interface{}{
			"waha_whatsapp_jid":      contactJID,
			"waha_avatar_checked_at": time.Now().UTC().Format(time.RFC3339),
		}
		_ = s.client.UpdateContactAttributes(contact.ID, contactJID, attrs, isGroup)
		return nil
	}

	newHash := sha256Hex(imgData)

	oldHash := ""
	if contact.CustomAttributes != nil {
		if v, ok := contact.CustomAttributes["waha_avatar_hash"].(string); ok {
			oldHash = v
		}
	}

	if oldHash != "" && oldHash == newHash {
		attrs := map[string]interface{}{
			"waha_whatsapp_jid":      contactJID,
			"waha_avatar_checked_at": time.Now().UTC().Format(time.RFC3339),
		}
		_ = s.client.UpdateContactAttributes(contact.ID, contactJID, attrs, isGroup)

		return nil
	}

	if err := s.client.UpdateContactAvatar(contact.ID, imgData); err != nil {
		return err
	}

	attrs := map[string]interface{}{
		"waha_whatsapp_jid":      contactJID,
		"waha_avatar_hash":       newHash,
		"waha_avatar_checked_at": time.Now().UTC().Format(time.RFC3339),
	}
	_ = s.client.UpdateContactAttributes(contact.ID, contactJID, attrs, isGroup)


	logrus.Infof("Chatwoot Sync: avatar updated jid=%s contact_id=%d", contactJID, contact.ID)
	return nil
}
