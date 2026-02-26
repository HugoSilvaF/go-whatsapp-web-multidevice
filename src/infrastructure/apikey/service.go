package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	keyPrefix  = "gowa"
	timeLayout = time.RFC3339
)

type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CreateParams struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

type ValidationResult struct {
	KeyID  string
	Scopes []string
}

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) InitializeSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS api_keys (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  key_hash TEXT NOT NULL,
  scopes TEXT NOT NULL,
  expires_at DATETIME,
  revoked_at DATETIME,
  last_used_at DATETIME,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_api_keys_revoked_at ON api_keys(revoked_at);
CREATE INDEX IF NOT EXISTS idx_api_keys_expires_at ON api_keys(expires_at);
`)
	return err
}

func (s *Service) CreateKey(ctx context.Context, p CreateParams) (*APIKey, string, error) {
	if strings.TrimSpace(p.Name) == "" {
		return nil, "", fmt.Errorf("name is required")
	}
	scopes := normalizeScopes(p.Scopes)
	if len(scopes) == 0 {
		return nil, "", fmt.Errorf("at least one scope is required")
	}

	id := uuid.NewString()
	plain, err := generatePlainKey(id)
	if err != nil {
		return nil, "", err
	}

	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO api_keys (id, name, key_hash, scopes, expires_at, revoked_at, last_used_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, NULL, NULL, ?, ?)
`, id, strings.TrimSpace(p.Name), hashKey(plain), strings.Join(scopes, ","), nullableTime(p.ExpiresAt), now, now)
	if err != nil {
		return nil, "", err
	}

	return &APIKey{
		ID:        id,
		Name:      strings.TrimSpace(p.Name),
		Scopes:    scopes,
		ExpiresAt: p.ExpiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}, plain, nil
}

func (s *Service) ListKeys(ctx context.Context, includeRevoked bool) ([]APIKey, error) {
	query := `
SELECT id, name, scopes, expires_at, revoked_at, last_used_at, created_at, updated_at
FROM api_keys
`
	if !includeRevoked {
		query += " WHERE revoked_at IS NULL"
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]APIKey, 0)
	for rows.Next() {
		var (
			k                          APIKey
			scopes                     string
			expiresAt, revokedAt, used sql.NullString
			createdAt, updatedAt       string
		)
		if err := rows.Scan(&k.ID, &k.Name, &scopes, &expiresAt, &revokedAt, &used, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		k.Scopes = splitScopes(scopes)
		k.ExpiresAt = parseNullTime(expiresAt)
		k.RevokedAt = parseNullTime(revokedAt)
		k.LastUsedAt = parseNullTime(used)
		k.CreatedAt = mustParseTime(createdAt)
		k.UpdatedAt = mustParseTime(updatedAt)
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Service) RevokeKey(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE api_keys
SET revoked_at = ?, updated_at = ?
WHERE id = ? AND revoked_at IS NULL
`, time.Now().UTC(), time.Now().UTC(), strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("api key not found")
	}
	return nil
}

func (s *Service) RotateKey(ctx context.Context, id string) (*APIKey, string, error) {
	keyID := strings.TrimSpace(id)
	metadata, err := s.getKeyByID(ctx, keyID)
	if err != nil {
		return nil, "", err
	}
	if metadata.RevokedAt != nil {
		return nil, "", fmt.Errorf("cannot rotate revoked key")
	}

	plain, err := generatePlainKey(keyID)
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE api_keys
SET key_hash = ?, updated_at = ?, revoked_at = NULL
WHERE id = ?
`, hashKey(plain), now, keyID)
	if err != nil {
		return nil, "", err
	}
	metadata.UpdatedAt = now
	return metadata, plain, nil
}

func (s *Service) Authenticate(ctx context.Context, rawKey string) (*ValidationResult, error) {
	keyID, err := parseKeyID(rawKey)
	if err != nil {
		return nil, err
	}

	var (
		keyHash   string
		scopesCSV string
		expiresAt sql.NullString
		revokedAt sql.NullString
	)
	err = s.db.QueryRowContext(ctx, `
SELECT key_hash, scopes, expires_at, revoked_at
FROM api_keys
WHERE id = ?
`, keyID).Scan(&keyHash, &scopesCSV, &expiresAt, &revokedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, err
	}

	if revokedAt.Valid {
		return nil, fmt.Errorf("api key revoked")
	}
	if exp := parseNullTime(expiresAt); exp != nil && time.Now().UTC().After(*exp) {
		return nil, fmt.Errorf("api key expired")
	}
	if !secureEqualHash(hashKey(rawKey), keyHash) {
		return nil, fmt.Errorf("invalid api key")
	}

	_, _ = s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ?, updated_at = ? WHERE id = ?`, time.Now().UTC(), time.Now().UTC(), keyID)

	return &ValidationResult{
		KeyID:  keyID,
		Scopes: splitScopes(scopesCSV),
	}, nil
}

func (s *Service) getKeyByID(ctx context.Context, id string) (*APIKey, error) {
	var (
		k                          APIKey
		scopes                     string
		expiresAt, revokedAt, used sql.NullString
		createdAt, updatedAt       string
	)
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, scopes, expires_at, revoked_at, last_used_at, created_at, updated_at
FROM api_keys
WHERE id = ?
`, id).Scan(&k.ID, &k.Name, &scopes, &expiresAt, &revokedAt, &used, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, err
	}
	k.Scopes = splitScopes(scopes)
	k.ExpiresAt = parseNullTime(expiresAt)
	k.RevokedAt = parseNullTime(revokedAt)
	k.LastUsedAt = parseNullTime(used)
	k.CreatedAt = mustParseTime(createdAt)
	k.UpdatedAt = mustParseTime(updatedAt)
	return &k, nil
}

func generatePlainKey(id string) (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("failed to generate api key secret: %w", err)
	}
	return fmt.Sprintf("%s.%s.%s", keyPrefix, id, base64.RawURLEncoding.EncodeToString(secret)), nil
}

func parseKeyID(raw string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), ".", 3)
	if len(parts) != 3 || parts[0] != keyPrefix || parts[1] == "" || parts[2] == "" {
		return "", fmt.Errorf("invalid api key format")
	}
	return parts[1], nil
}

func hashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normalizeScopes(in []string) []string {
	seen := make(map[string]struct{})
	scopes := make([]string, 0, len(in))
	for _, scope := range in {
		s := strings.TrimSpace(strings.ToLower(scope))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)
	return scopes
}

func splitScopes(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func nullableTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.UTC().Format(timeLayout)
}

func parseNullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || strings.TrimSpace(ns.String) == "" {
		return nil
	}
	t := mustParseTime(ns.String)
	return &t
}

func mustParseTime(raw string) time.Time {
	if t, err := time.Parse(timeLayout, raw); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02 15:04:05", raw); err == nil {
		return t.UTC()
	}
	return time.Now().UTC()
}

func secureEqualHash(a, b string) bool {
	if len(a) != len(b) || a == "" || b == "" {
		return false
	}
	// compare hashes without leaking timing details
	return subtleCompare(a, b)
}

func subtleCompare(a, b string) bool {
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
