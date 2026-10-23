package handlers

import "testing"

func TestNextWorkflowStateTransitions(t *testing.T) {
	tests := []struct {
		name          string
		current       string
		selfComplete  bool
		tasksComplete bool
		want          string
		wantErr       error
	}{
		{name: "draft starts self evaluation", current: workflowStateDraft, want: workflowStateSelfEvaluating},
		{name: "self incomplete", current: workflowStateSelfEvaluating, selfComplete: false, want: workflowStateSelfEvaluating, wantErr: errSelfEvaluationIncomplete},
		{name: "self complete", current: workflowStateSelfEvaluating, selfComplete: true, want: workflowStateScoring},
		{name: "scoring incomplete", current: workflowStateScoring, selfComplete: true, tasksComplete: false, want: workflowStateScoring, wantErr: errRequiredTasksIncomplete},
		{name: "scoring complete", current: workflowStateScoring, selfComplete: true, tasksComplete: true, want: workflowStateAwaitingEmployeeConfirmation},
		{name: "optional review complete", current: workflowStateAwaitingReview, selfComplete: true, tasksComplete: true, want: workflowStateAwaitingEmployeeConfirmation},
		{name: "confirmation is terminal for advance", current: workflowStateAwaitingEmployeeConfirmation, selfComplete: true, tasksComplete: true, want: workflowStateAwaitingEmployeeConfirmation, wantErr: errWorkflowTerminal},
		{name: "completed terminal", current: workflowStateCompleted, selfComplete: true, tasksComplete: true, want: workflowStateCompleted, wantErr: errWorkflowTerminal},
		{name: "rejected terminal", current: workflowStateRejected, selfComplete: true, tasksComplete: true, want: workflowStateRejected, wantErr: errWorkflowTerminal},
		{name: "cancelled terminal", current: workflowStateCancelled, selfComplete: true, tasksComplete: true, want: workflowStateCancelled, wantErr: errWorkflowTerminal},
		{name: "legacy jump rejected", current: "manager_evaluated", selfComplete: true, tasksComplete: true, want: "manager_evaluated", wantErr: errWorkflowStateConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := nextWorkflowState(test.current, test.selfComplete, test.tasksComplete)
			if got != test.want || err != test.wantErr {
				t.Fatalf("got=(%s,%v) want=(%s,%v)", got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestConfirmationStatesSupportLegacyAndNewWorkflow(t *testing.T) {
	for _, status := range []string{"pending_confirm", workflowStateAwaitingEmployeeConfirmation} {
		if !canConfirmEvaluationState(status) {
			t.Fatalf("status %q should allow confirmation", status)
		}
	}
	for _, status := range []string{workflowStateScoring, workflowStateCompleted, workflowStateRejected} {
		if canConfirmEvaluationState(status) {
			t.Fatalf("status %q must not allow confirmation", status)
		}
	}
}
