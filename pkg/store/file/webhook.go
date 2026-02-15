//go:build filestore

package file

import (
	"context"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
	"github.com/google/uuid"
)

// GetWebhookByID returns a webhook by its ID.
func (s *FileStore) GetWebhookByID(ctx context.Context, h db.Handler, repoID int64, id int64) (models.Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, r := range s.repos {
		if hashUsername(r.name) == repoID {
			if int(id) < len(r.webhooks) {
				wh := r.webhooks[id]
				return models.Webhook{
					ID:          id,
					RepoID:      repoID,
					URL:         wh.url,
					Secret:      wh.secret,
					ContentType: 1,
					Active:      wh.active,
				}, nil
			}
		}
	}

	return models.Webhook{}, db.ErrRecordNotFound
}

// GetWebhooksByRepoID returns all webhooks for a repository.
func (s *FileStore) GetWebhooksByRepoID(ctx context.Context, h db.Handler, repoID int64) ([]models.Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, r := range s.repos {
		if hashUsername(r.name) == repoID {
			webhooks := make([]models.Webhook, 0, len(r.webhooks))
			for i, wh := range r.webhooks {
				webhooks = append(webhooks, models.Webhook{
					ID:          int64(i),
					RepoID:      repoID,
					URL:         wh.url,
					Secret:      wh.secret,
					ContentType: 1,
					Active:      wh.active,
				})
			}
			return webhooks, nil
		}
	}

	return nil, db.ErrRecordNotFound
}

// GetWebhooksByRepoIDWhereEvent returns all webhooks for a repository where event is in the events.
func (s *FileStore) GetWebhooksByRepoIDWhereEvent(ctx context.Context, h db.Handler, repoID int64, events []int) ([]models.Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, r := range s.repos {
		if hashUsername(r.name) == repoID {
			webhooks := make([]models.Webhook, 0)
			for i, wh := range r.webhooks {
				if hasEvent(wh.events, events) {
					webhooks = append(webhooks, models.Webhook{
						ID:          int64(i),
						RepoID:      repoID,
						URL:         wh.url,
						Secret:      wh.secret,
						ContentType: 1,
						Active:      wh.active,
					})
				}
			}
			return webhooks, nil
		}
	}

	return nil, db.ErrRecordNotFound
}

// CreateWebhook creates a webhook.
func (s *FileStore) CreateWebhook(ctx context.Context, h db.Handler, repoID int64, url string, secret string, contentType int, active bool) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range s.repos {
		if hashUsername(r.name) == repoID {
			id := int64(len(r.webhooks))
			r.webhooks = append(r.webhooks, webhookInfo{
				url:    url,
				secret: secret,
				active: active,
				events: []int{},
			})

			// Save metadata
			_ = s.saveRepoMeta(r.path, &repoMeta{
				Description:   r.description,
				Private:       r.private,
				ProjectName:   r.projectName,
				Hidden:        r.hidden,
				Mirror:        r.mirror,
				Collaborators: collabMapToMeta(r.collabs),
				Webhooks:      webhookInfoToMeta(r.webhooks),
			})

			return id, nil
		}
	}

	return 0, db.ErrRecordNotFound
}

// UpdateWebhookByID updates a webhook by its ID.
func (s *FileStore) UpdateWebhookByID(ctx context.Context, h db.Handler, repoID int64, id int64, url string, secret string, contentType int, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range s.repos {
		if hashUsername(r.name) == repoID {
			if int(id) < len(r.webhooks) {
				r.webhooks[id] = webhookInfo{
					url:    url,
					secret: secret,
					active: active,
					events: r.webhooks[id].events,
				}

				// Save metadata
				return s.saveRepoMeta(r.path, &repoMeta{
					Description:   r.description,
					Private:       r.private,
					ProjectName:   r.projectName,
					Hidden:        r.hidden,
					Mirror:        r.mirror,
					Collaborators: collabMapToMeta(r.collabs),
					Webhooks:      webhookInfoToMeta(r.webhooks),
				})
			}
		}
	}

	return db.ErrRecordNotFound
}

// DeleteWebhookByID deletes a webhook by its ID.
func (s *FileStore) DeleteWebhookByID(ctx context.Context, h db.Handler, id int64) error {
	// Not supported - need repo context
	return ErrNotSupported
}

// DeleteWebhookForRepoByID deletes a webhook for a repository by its ID.
func (s *FileStore) DeleteWebhookForRepoByID(ctx context.Context, h db.Handler, repoID int64, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range s.repos {
		if hashUsername(r.name) == repoID {
			if int(id) < len(r.webhooks) {
				r.webhooks = append(r.webhooks[:id], r.webhooks[id+1:]...)

				// Save metadata
				return s.saveRepoMeta(r.path, &repoMeta{
					Description:   r.description,
					Private:       r.private,
					ProjectName:   r.projectName,
					Hidden:        r.hidden,
					Mirror:        r.mirror,
					Collaborators: collabMapToMeta(r.collabs),
					Webhooks:      webhookInfoToMeta(r.webhooks),
				})
			}
		}
	}

	return db.ErrRecordNotFound
}

// GetWebhookEventByID returns a webhook event by its ID.
func (s *FileStore) GetWebhookEventByID(ctx context.Context, h db.Handler, id int64) (models.WebhookEvent, error) {
	return models.WebhookEvent{}, ErrNotSupported
}

// GetWebhookEventsByWebhookID returns all webhook events for a webhook.
func (s *FileStore) GetWebhookEventsByWebhookID(ctx context.Context, h db.Handler, webhookID int64) ([]models.WebhookEvent, error) {
	return nil, ErrNotSupported
}

// CreateWebhookEvents creates webhook events for a webhook.
func (s *FileStore) CreateWebhookEvents(ctx context.Context, h db.Handler, webhookID int64, events []int) error {
	return ErrNotSupported
}

// DeleteWebhookEventsByID deletes all webhook events for a webhook.
func (s *FileStore) DeleteWebhookEventsByID(ctx context.Context, h db.Handler, ids []int64) error {
	return ErrNotSupported
}

// GetWebhookDeliveryByID returns a webhook delivery by its ID.
func (s *FileStore) GetWebhookDeliveryByID(ctx context.Context, h db.Handler, webhookID int64, id uuid.UUID) (models.WebhookDelivery, error) {
	return models.WebhookDelivery{}, ErrNotSupported
}

// GetWebhookDeliveriesByWebhookID returns all webhook deliveries for a webhook.
func (s *FileStore) GetWebhookDeliveriesByWebhookID(ctx context.Context, h db.Handler, webhookID int64) ([]models.WebhookDelivery, error) {
	return nil, ErrNotSupported
}

// ListWebhookDeliveriesByWebhookID returns all webhook deliveries for a webhook.
func (s *FileStore) ListWebhookDeliveriesByWebhookID(ctx context.Context, h db.Handler, webhookID int64) ([]models.WebhookDelivery, error) {
	return nil, ErrNotSupported
}

// CreateWebhookDelivery creates a webhook delivery.
func (s *FileStore) CreateWebhookDelivery(ctx context.Context, h db.Handler, id uuid.UUID, webhookID int64, event int, url string, method string, requestError error, requestHeaders string, requestBody string, responseStatus int, responseHeaders string, responseBody string) error {
	return ErrNotSupported
}

// DeleteWebhookDeliveryByID deletes a webhook delivery by its ID.
func (s *FileStore) DeleteWebhookDeliveryByID(ctx context.Context, h db.Handler, webhookID int64, id uuid.UUID) error {
	return ErrNotSupported
}

// hasEvent checks if any of the events are in the webhook events.
func hasEvent(webhookEvents []int, events []int) bool {
	for _, e := range events {
		for _, we := range webhookEvents {
			if e == we {
				return true
			}
		}
	}
	return false
}
