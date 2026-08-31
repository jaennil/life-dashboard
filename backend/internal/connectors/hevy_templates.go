package connectors

import (
	"context"
	"encoding/json"
	"fmt"
)

// Templates page at up to 100 per request, unlike workouts and routines which
// cap at 10, so the whole catalogue of roughly 460 entries costs five calls
// instead of forty-six.
const hevyTemplatePageSize = 100

type hevyExerciseTemplatesResponse struct {
	Page      int                    `json:"page"`
	PageCount int                    `json:"page_count"`
	Templates []hevyExerciseTemplate `json:"exercise_templates"`
}

type hevyExerciseTemplate struct {
	ID                    string   `json:"id"`
	Title                 string   `json:"title"`
	Type                  string   `json:"type"`
	PrimaryMuscleGroup    string   `json:"primary_muscle_group"`
	SecondaryMuscleGroups []string `json:"secondary_muscle_groups"`
	Equipment             string   `json:"equipment"`
	IsCustom              bool     `json:"is_custom"`
}

// syncExerciseTemplates refreshes the local catalogue used to resolve a dictated
// exercise name to the exercise_template_id that writing a workout requires.
//
// Nothing is pruned here. A template that disappears from the catalogue may
// still be referenced by workouts already logged against it, and losing the row
// would leave that history unresolvable.
func (h *HevyConnector) syncExerciseTemplates(ctx context.Context, userID, apiKey string) error {
	page := 1
	total := 0

	for {
		resp, err := h.fetchExerciseTemplatesPage(ctx, apiKey, page)
		if err != nil {
			return fmt.Errorf("fetch exercise templates page %d: %w", page, err)
		}

		for i := range resp.Templates {
			if err := h.upsertExerciseTemplate(ctx, userID, &resp.Templates[i]); err != nil {
				return fmt.Errorf("upsert exercise template %s: %w", resp.Templates[i].ID, err)
			}
		}

		total += len(resp.Templates)
		h.logger.Debug().Int("page", page).Int("page_count", resp.PageCount).Int("synced", total).
			Msg("exercise template page synced")

		if resp.PageCount <= 0 || page >= resp.PageCount || len(resp.Templates) == 0 {
			break
		}
		page++
	}

	h.logger.Info().Int("total", total).Msg("exercise templates sync complete")
	return nil
}

func (h *HevyConnector) fetchExerciseTemplatesPage(ctx context.Context, apiKey string, page int) (*hevyExerciseTemplatesResponse, error) {
	url := fmt.Sprintf("%s/exercise_templates?page=%d&pageSize=%d", hevyBaseURL, page, hevyTemplatePageSize)
	return doRequest[hevyExerciseTemplatesResponse](ctx, h.client, apiKey, url)
}

// upsertExerciseTemplate stores one template. Only a custom template records an
// owner: built-ins are identical for every account, so keeping them user-scoped
// would duplicate the whole catalogue per user for no gain.
func (h *HevyConnector) upsertExerciseTemplate(ctx context.Context, userID string, template *hevyExerciseTemplate) error {
	raw, err := json.Marshal(template)
	if err != nil {
		return err
	}

	owner := hevyTemplateOwner(userID, template.IsCustom)

	_, err = h.db.Exec(ctx, `
		INSERT INTO hevy_exercise_templates (
			id, owner_user_id, title, type, primary_muscle_group,
			secondary_muscle_groups, equipment, is_custom, raw_payload, updated_at
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''), $8, $9::jsonb, NOW())
		ON CONFLICT (id) DO UPDATE SET
			owner_user_id           = COALESCE(EXCLUDED.owner_user_id, hevy_exercise_templates.owner_user_id),
			title                   = EXCLUDED.title,
			type                    = EXCLUDED.type,
			primary_muscle_group    = EXCLUDED.primary_muscle_group,
			secondary_muscle_groups = EXCLUDED.secondary_muscle_groups,
			equipment               = EXCLUDED.equipment,
			is_custom               = EXCLUDED.is_custom,
			raw_payload             = EXCLUDED.raw_payload,
			updated_at              = NOW()
	`, template.ID, owner, template.Title, template.Type, template.PrimaryMuscleGroup,
		template.SecondaryMuscleGroups, template.Equipment, template.IsCustom, raw)
	if err != nil {
		return fmt.Errorf("upsert template: %w", err)
	}
	return nil
}

// hevyTemplateOwner scopes a template to a user only when it is custom. Built-in
// templates are identical for every account, so giving them an owner would both
// duplicate the catalogue per user and make one user's sync overwrite the
// ownership recorded by another's.
func hevyTemplateOwner(userID string, isCustom bool) *string {
	if !isCustom {
		return nil
	}
	return &userID
}
