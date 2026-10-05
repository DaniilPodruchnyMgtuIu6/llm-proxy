package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrPresetNotFound      = errors.New("preset not found")
	ErrPresetExists        = errors.New("preset already exists")
	ErrPresetSystemProtect = errors.New("system preset cannot be deleted")
	ErrPresetInvalidSlug   = errors.New("invalid preset slug")
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Preset is a named chat configuration stored in PostgreSQL.
type Preset struct {
	Slug             string    `json:"slug"`
	Title            string    `json:"title"`
	Description      string    `json:"description,omitempty"`
	Model            string    `json:"model"`
	Temperature      *float64  `json:"temperature,omitempty"`
	TopP             *float64  `json:"top_p,omitempty"`
	TopK             *int      `json:"top_k,omitempty"`
	MaxTokens        *int      `json:"max_tokens,omitempty"`
	PresencePenalty  *float64  `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64  `json:"frequency_penalty,omitempty"`
	IsSystem         bool      `json:"is_system"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func NormalizePresetSlug(slug string) (string, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugRe.MatchString(slug) {
		return "", fmt.Errorf("%w: use lowercase letters, digits, _ or - (1..64), starting with alnum", ErrPresetInvalidSlug)
	}
	return slug, nil
}

func (s *Store) ListPresets(ctx context.Context) ([]Preset, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT slug, title, description, model, temperature, top_p, top_k, max_tokens,
       presence_penalty, frequency_penalty, is_system, created_at, updated_at
FROM presets
ORDER BY is_system DESC, slug ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Preset, 0)
	for rows.Next() {
		p, err := scanPreset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPreset(ctx context.Context, slug string) (Preset, error) {
	slug, err := NormalizePresetSlug(slug)
	if err != nil {
		return Preset{}, err
	}
	if s == nil {
		return Preset{}, ErrPresetNotFound
	}
	row := s.db.QueryRowContext(ctx, `
SELECT slug, title, description, model, temperature, top_p, top_k, max_tokens,
       presence_penalty, frequency_penalty, is_system, created_at, updated_at
FROM presets WHERE slug = $1`, slug)
	p, err := scanPreset(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Preset{}, ErrPresetNotFound
	}
	return p, err
}

func (s *Store) CreatePreset(ctx context.Context, p Preset) (Preset, error) {
	slug, err := NormalizePresetSlug(p.Slug)
	if err != nil {
		return Preset{}, err
	}
	p.Slug = slug
	if strings.TrimSpace(p.Title) == "" {
		p.Title = p.Slug
	}
	if strings.TrimSpace(p.Model) == "" {
		p.Model = "auto"
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO presets (
  slug, title, description, model, temperature, top_p, top_k, max_tokens,
  presence_penalty, frequency_penalty, is_system, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)`,
		p.Slug, p.Title, nullStr(p.Description), p.Model,
		nullFloat(p.Temperature), nullFloat(p.TopP), nullInt(p.TopK), nullInt(p.MaxTokens),
		nullFloat(p.PresencePenalty), nullFloat(p.FrequencyPenalty),
		p.IsSystem, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(err.Error(), "unique") {
			return Preset{}, ErrPresetExists
		}
		return Preset{}, err
	}
	return s.GetPreset(ctx, p.Slug)
}

func (s *Store) UpdatePreset(ctx context.Context, slug string, p Preset) (Preset, error) {
	slug, err := NormalizePresetSlug(slug)
	if err != nil {
		return Preset{}, err
	}
	cur, err := s.GetPreset(ctx, slug)
	if err != nil {
		return Preset{}, err
	}
	if strings.TrimSpace(p.Title) == "" {
		p.Title = cur.Title
	}
	if strings.TrimSpace(p.Model) == "" {
		p.Model = "auto"
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
UPDATE presets SET
  title = $2,
  description = $3,
  model = $4,
  temperature = $5,
  top_p = $6,
  top_k = $7,
  max_tokens = $8,
  presence_penalty = $9,
  frequency_penalty = $10,
  updated_at = $11
WHERE slug = $1`,
		slug, p.Title, nullStr(p.Description), p.Model,
		nullFloat(p.Temperature), nullFloat(p.TopP), nullInt(p.TopK), nullInt(p.MaxTokens),
		nullFloat(p.PresencePenalty), nullFloat(p.FrequencyPenalty), now,
	)
	if err != nil {
		return Preset{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Preset{}, ErrPresetNotFound
	}
	return s.GetPreset(ctx, slug)
}

func (s *Store) DeletePreset(ctx context.Context, slug string) error {
	slug, err := NormalizePresetSlug(slug)
	if err != nil {
		return err
	}
	cur, err := s.GetPreset(ctx, slug)
	if err != nil {
		return err
	}
	if cur.IsSystem {
		return ErrPresetSystemProtect
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM presets WHERE slug = $1 AND is_system = FALSE`, slug)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPresetNotFound
	}
	return nil
}

// EnsureDefaultPreset creates/updates the system default preset from optional seed values.
func (s *Store) EnsureDefaultPreset(ctx context.Context, seed Preset) error {
	if s == nil {
		return nil
	}
	cur, err := s.GetPreset(ctx, "default")
	if errors.Is(err, ErrPresetNotFound) {
		seed.Slug = "default"
		seed.IsSystem = true
		if seed.Title == "" {
			seed.Title = "Default"
		}
		if seed.Model == "" {
			seed.Model = "auto"
		}
		_, err = s.CreatePreset(ctx, seed)
		return err
	}
	if err != nil {
		return err
	}
	// Keep system flag; refresh sampling from seed only when seed has meaningful data
	// and current is still bare auto with no params (first boot after migrate).
	if seed.Model != "" && cur.Model == "auto" &&
		cur.Temperature == nil && cur.TopP == nil && cur.MaxTokens == nil &&
		(seed.Temperature != nil || seed.TopP != nil || seed.MaxTokens != nil || seed.Model != "auto") {
		seed.Title = cur.Title
		if seed.Title == "" {
			seed.Title = "Default"
		}
		_, err = s.UpdatePreset(ctx, "default", seed)
		return err
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPreset(sc scanner) (Preset, error) {
	var p Preset
	var desc sql.NullString
	var temp, topP, presence, frequency sql.NullFloat64
	var topK, maxTokens sql.NullInt64
	err := sc.Scan(
		&p.Slug, &p.Title, &desc, &p.Model,
		&temp, &topP, &topK, &maxTokens,
		&presence, &frequency,
		&p.IsSystem, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Preset{}, err
	}
	if desc.Valid {
		p.Description = desc.String
	}
	if temp.Valid {
		v := temp.Float64
		p.Temperature = &v
	}
	if topP.Valid {
		v := topP.Float64
		p.TopP = &v
	}
	if topK.Valid {
		v := int(topK.Int64)
		p.TopK = &v
	}
	if maxTokens.Valid {
		v := int(maxTokens.Int64)
		p.MaxTokens = &v
	}
	if presence.Valid {
		v := presence.Float64
		p.PresencePenalty = &v
	}
	if frequency.Valid {
		v := frequency.Float64
		p.FrequencyPenalty = &v
	}
	return p, nil
}

func nullStr(s string) any {
	if s == "" {
		return ""
	}
	return s
}

func nullFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
