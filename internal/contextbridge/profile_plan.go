package contextbridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/luoyif/memory-harness/internal/profile"
)

func projectionCost(value profile.Projection) (int, int) {
	raw, _ := json.Marshal(value)
	chars := len([]rune(string(raw)))
	return chars, max(1, (chars+3)/4)
}

func profileSourceRefs(value profile.Projection) []string {
	seen := map[string]bool{}
	refs := []string{}
	for _, block := range value.Blocks {
		for _, ref := range block.SourceRefs {
			ref = strings.TrimSpace(ref)
			if ref != "" && !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	sort.Strings(refs)
	return refs
}

func (s *Service) profilePlanItems(ctx context.Context, planID, projectID string, policy compiledContextPolicy) ([]ContextPlanItem, int, int, error) {
	if s.profiles == nil || !policy.Enabled || !policy.ProfileEnabled {
		return nil, 0, 0, nil
	}
	items := []ContextPlanItem{}
	usedChars, usedTokens := 0, 0
	for _, viewKind := range policy.ProfileViews {
		projection, object, err := s.profiles.Projection(ctx, projectID, viewKind)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, 0, 0, err
		}
		if object.ProjectID != projectID || object.Status != "active" {
			continue
		}
		chars, tokens := projectionCost(projection)
		if usedChars+chars > policy.ProfileMaxChars || usedTokens+tokens > policy.ProfileMaxTokens {
			continue
		}
		item := ContextPlanItem{
			SourceKind: "object", SourceID: object.ObjectID, Revision: object.CurrentRevision,
			ContentHash: object.Revision.ContentHash, ProjectID: projectID,
			ReasonCodes: []string{"context_profile", "profile:" + viewKind},
			Priority:    max(1, 100-len(items)*5), TokenEstimate: tokens,
			Presentation: policy.ProfilePresentation, SourceRefs: profileSourceRefs(projection),
		}
		item.ItemID = stableContextID("ctxitem-", planID, item.SourceKind, item.SourceID, fmt.Sprint(item.Revision), item.ContentHash)
		items = append(items, item)
		usedChars += chars
		usedTokens += tokens
		if len(items) >= policy.MaxProfiles {
			break
		}
	}
	return items, usedTokens, usedChars, nil
}
