package flighttask

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

const (
	AutomaticDispatchMachineUse = "automatic-task-dispatch"
	AutomaticDispatchActorID    = "system:auto-dispatch"
)

// AutomaticTaskDispatcher is deliberately a port. The current selector is a
// deterministic baseline only; the full scheduling policy (fairness, shifts,
// workload and handover rules) can be replaced after business confirmation.
type AutomaticTaskDispatcher interface {
	Dispatch(context.Context, ArrivalResult) (ConfirmationResult, error)
}

type CandidateSelector interface {
	Select(context.Context, ArrivalResult) (CandidateView, error)
}

// RankedCandidateSelector preserves the candidate snapshot order produced by
// task generation. It is safe, deterministic and intentionally small enough
// to be replaced without changing the task/assignment transaction boundary.
type RankedCandidateSelector struct{}

func (RankedCandidateSelector) Select(ctx context.Context, result ArrivalResult) (CandidateView, error) {
	if err := ctx.Err(); err != nil {
		return CandidateView{}, err
	}
	candidates := append([]CandidateView(nil), result.Candidates...)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Rank == candidates[j].Rank {
			return candidates[i].PublicID < candidates[j].PublicID
		}
		return candidates[i].Rank < candidates[j].Rank
	})
	for _, candidate := range candidates {
		if candidate.PublicID != "" {
			return candidate, nil
		}
	}
	return CandidateView{}, fmt.Errorf("no candidate is available for automatic dispatch")
}

type AutomaticDispatcher struct {
	confirmation *ConfirmationService
	selector     CandidateSelector
}

func NewAutomaticDispatcher(confirmation *ConfirmationService, selector CandidateSelector) *AutomaticDispatcher {
	if selector == nil {
		selector = RankedCandidateSelector{}
	}
	return &AutomaticDispatcher{confirmation: confirmation, selector: selector}
}

func (d *AutomaticDispatcher) Dispatch(ctx context.Context, result ArrivalResult) (ConfirmationResult, error) {
	if d == nil || d.confirmation == nil {
		return ConfirmationResult{}, fmt.Errorf("automatic task dispatcher is not configured")
	}
	if result.TaskPublicID == "" || len(result.Candidates) == 0 {
		return ConfirmationResult{}, fmt.Errorf("automatic dispatch requires a task and candidates")
	}
	first, err := d.selector.Select(ctx, result)
	if err != nil {
		return ConfirmationResult{}, err
	}
	ordered := make([]CandidateView, 0, len(result.Candidates))
	ordered = append(ordered, first)
	for _, candidate := range result.Candidates {
		if candidate.PublicID != first.PublicID {
			ordered = append(ordered, candidate)
		}
	}
	principal := security.Principal{Type: security.MachinePrincipal, PublicID: AutomaticDispatchActorID, MachineUse: AutomaticDispatchMachineUse}
	var lastErr error
	for _, candidate := range ordered {
		confirmationID := "auto-dispatch:" + result.TaskPublicID + ":" + candidate.PublicID
		assigned, confirmErr := d.confirmation.ConfirmTask(ctx, ConfirmationInput{
			Principal:           principal,
			TaskPublicID:        result.TaskPublicID,
			CandidatePublicID:   candidate.PublicID,
			ConfirmationID:      confirmationID,
			ExpectedTaskVersion: 0,
			TraceID:             "auto-dispatch:" + result.TaskPublicID,
		})
		if confirmErr == nil {
			return assigned, nil
		}
		if CodeOf(confirmErr) == ResultCandidateNoLongerEligible {
			lastErr = confirmErr
			continue
		}
		if CodeOf(confirmErr) == ResultTaskAlreadyAssigned {
			return assigned, nil
		}
		return ConfirmationResult{}, confirmErr
	}
	if lastErr != nil {
		return ConfirmationResult{}, errors.Join(fmt.Errorf("all automatic dispatch candidates became unavailable"), lastErr)
	}
	return ConfirmationResult{}, fmt.Errorf("automatic dispatch did not select a candidate")
}
