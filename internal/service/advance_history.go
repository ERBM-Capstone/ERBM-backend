package service

import (
	"context"
	"sort"
	"strings"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
)

func (s *AdvanceService) History(ctx context.Context, userID int64, role string, id int64) ([]model.AdvanceHistoryItem, error) {
	if userID <= 0 {
		return nil, dto.ErrUnauthorized
	}
	role = strings.ToUpper(strings.TrimSpace(role))
	switch role {
	case roleAdmin, roleAccounting, rolePM, rolePC:
	default:
		return nil, dto.ErrForbidden
	}
	if id <= 0 {
		return nil, dto.ErrBadRequest
	}
	history, err := s.repo.GetHistory(ctx, id)
	if err != nil {
		return nil, advanceDetailError(err)
	}
	if history == nil {
		return nil, dto.ErrInternalServer
	}
	if err := authorizeAdvanceRead(&history.Advance, userID, role); err != nil {
		return nil, err
	}
	events := append([]model.AdvanceHistoryItem{}, history.Events...)
	var submissions []int
	for i, event := range events {
		if event.Action == model.AdvanceActionSubmit {
			submissions = append(submissions, i)
		}
	}
	// Revision numbers follow event IDs, independent of display timestamps.
	// Keep the repository's newest-first timeline order unchanged.
	sort.Slice(submissions, func(i, j int) bool {
		return events[submissions[i]].ID < events[submissions[j]].ID
	})
	for i, index := range submissions {
		revision := int64(i + 1)
		events[index].Revision = &revision
	}
	return events, nil
}
