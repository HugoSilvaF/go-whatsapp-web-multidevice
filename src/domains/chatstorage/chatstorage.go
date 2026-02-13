package chatstorage

import (
	"context"
	"database/sql"
	"time"
)

type PostgresRepository struct {
	DB *sql.DB
}

func (r *PostgresRepository) GetChatExportState(deviceID, chatJID string) (*ChatExportState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := r.DB.QueryRowContext(ctx, `
		SELECT device_id, chat_jid, last_exported_at, updated_at
		FROM chatwoot_export_state
		WHERE device_id = $1 AND chat_jid = $2
	`, deviceID, chatJID)

	var st ChatExportState
	err := row.Scan(&st.DeviceID, &st.ChatJID, &st.LastExportedAt, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (r *PostgresRepository) UpsertChatExportState(state *ChatExportState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO chatwoot_export_state (device_id, chat_jid, last_exported_at, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (device_id, chat_jid)
		DO UPDATE SET last_exported_at = EXCLUDED.last_exported_at, updated_at = NOW()
	`, state.DeviceID, state.ChatJID, state.LastExportedAt)
	return err
}

func (r *PostgresRepository) IsMessageExported(deviceID, chatJID, messageKey string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := r.DB.QueryRowContext(ctx, `
		SELECT 1
		FROM chatwoot_exported_messages
		WHERE device_id = $1 AND chat_jid = $2 AND message_key = $3
		LIMIT 1
	`, deviceID, chatJID, messageKey)

	var one int
	err := row.Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *PostgresRepository) MarkMessageExported(deviceID, chatJID, messageKey string, chatwootMessageID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO chatwoot_exported_messages (device_id, chat_jid, message_key, chatwoot_message_id, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (device_id, chat_jid, message_key)
		DO NOTHING
	`, deviceID, chatJID, messageKey, chatwootMessageID)
	return err
}

func (r *PostgresRepository) IsChatwootMessageFromUs(chatwootMessageID int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := r.DB.QueryRowContext(ctx, `
		SELECT 1
		FROM chatwoot_exported_messages
		WHERE chatwoot_message_id = $1
		LIMIT 1
	`, chatwootMessageID)

	var one int
	err := row.Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Chat represents a WhatsApp chat/conversation
type Chat struct {
	DeviceID            string    `db:"device_id"`
	JID                 string    `db:"jid"`
	Name                string    `db:"name"`
	LastMessageTime     time.Time `db:"last_message_time"`
	EphemeralExpiration uint32    `db:"ephemeral_expiration"`
	CreatedAt           time.Time `db:"created_at"`
	UpdatedAt           time.Time `db:"updated_at"`
}

// Message represents a WhatsApp message
type Message struct {
	ID            string    `db:"id"`
	ChatJID       string    `db:"chat_jid"`
	DeviceID      string    `db:"device_id"`
	Sender        string    `db:"sender"`
	Content       string    `db:"content"`
	Timestamp     time.Time `db:"timestamp"`
	IsFromMe      bool      `db:"is_from_me"`
	MediaType     string    `db:"media_type"`
	Filename      string    `db:"filename"`
	URL           string    `db:"url"`
	MediaKey      []byte    `db:"media_key"`
	FileSHA256    []byte    `db:"file_sha256"`
	FileEncSHA256 []byte    `db:"file_enc_sha256"`
	FileLength    uint64    `db:"file_length"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// MediaInfo represents downloadable media information
type MediaInfo struct {
	MessageID     string
	ChatJID       string
	MediaType     string
	Filename      string
	URL           string
	MediaKey      []byte
	FileSHA256    []byte
	FileEncSHA256 []byte
	FileLength    uint64
}

// DeviceRecord tracks a registered device for persistence purposes.
type DeviceRecord struct {
	DeviceID    string    `db:"device_id"`
	DisplayName string    `db:"display_name"`
	JID         string    `db:"jid"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// MessageFilter represents query filters for messages
type MessageFilter struct {
	DeviceID  string
	ChatJID   string
	Limit     int
	Offset    int
	StartTime *time.Time
	EndTime   *time.Time
	MediaOnly bool
	IsFromMe  *bool
}

// ChatFilter represents query filters for chats
type ChatFilter struct {
	DeviceID   string
	Limit      int
	Offset     int
	SearchName string
	HasMedia   bool
}
