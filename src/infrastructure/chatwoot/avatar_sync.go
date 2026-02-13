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
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", picInfo.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(imgData) == 0 {
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
	_ = s.client.UpdateContactAttributes(contact.ID, contactJID, attrs)

	logrus.Infof("Chatwoot Sync: avatar updated jid=%s contact_id=%d", contactJID, contact.ID)
	return nil
}
