package sirkel_repository

import (
	"context"
	"fmt"
	"testing"

	"sirkel-engine-lib/sirkel_domain"
)

func createTestProject(t *testing.T, handler *ProjectsRepositoryHandler, organizationID string) *sirkel_domain.Project {
	t.Helper()

	return mustSaveProject(t, handler, organizationID, "Test Project", sirkel_domain.ProjectStatePlanning)
}

func mustSaveGoal(t *testing.T, handler *GoalsRepositoryHandler, projectID, name string, state sirkel_domain.GoalState) *sirkel_domain.Goal {
	t.Helper()

	description := "test description"
	goal := &sirkel_domain.Goal{
		ProjectID:   projectID,
		Name:        name,
		Description: &description,
		State:       state,
	}
	if err := handler.SaveGoal(goal); err != nil {
		t.Fatalf("SaveGoal() error = %v", err)
	}

	return goal
}

func TestGoalsRepositoryHandler_SaveGoal_Insert(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	handler := NewGoalsRepositoryHandler(context.Background(), pool, nil)

	description := "Reach the moon"
	goal := &sirkel_domain.Goal{
		ProjectID:   project.ID,
		Name:        "Moonshot",
		Description: &description,
		State:       sirkel_domain.GoalStateActive,
	}

	if err := handler.SaveGoal(goal); err != nil {
		t.Fatalf("SaveGoal() error = %v", err)
	}

	if goal.ID == "" {
		t.Fatal("expected goal ID to be set after insert")
	}
	if goal.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set after insert")
	}
	if goal.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be set after insert")
	}
}

func TestGoalsRepositoryHandler_SaveGoal_Update(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	handler := NewGoalsRepositoryHandler(context.Background(), pool, nil)

	goal := mustSaveGoal(t, handler, project.ID, "Moonshot", sirkel_domain.GoalStateActive)

	goal.Name = "Moonshot v2"
	goal.State = sirkel_domain.GoalStateAchieved

	if err := handler.SaveGoal(goal); err != nil {
		t.Fatalf("SaveGoal() update error = %v", err)
	}

	fetched, err := handler.GetGoalByID(&goal.ID)
	if err != nil {
		t.Fatalf("GetGoalByID() error = %v", err)
	}
	if fetched == nil {
		t.Fatal("expected goal to exist after update")
	}
	if fetched.Name != "Moonshot v2" {
		t.Fatalf("expected name %q, got %q", "Moonshot v2", fetched.Name)
	}
	if fetched.State != sirkel_domain.GoalStateAchieved {
		t.Fatalf("expected state %q, got %q", sirkel_domain.GoalStateAchieved, fetched.State)
	}
}

func TestGoalsRepositoryHandler_SaveGoal_NullDescription(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	handler := NewGoalsRepositoryHandler(context.Background(), pool, nil)

	goal := &sirkel_domain.Goal{
		ProjectID: project.ID,
		Name:      "No description",
		State:     sirkel_domain.GoalStateActive,
	}

	if err := handler.SaveGoal(goal); err != nil {
		t.Fatalf("SaveGoal() error = %v", err)
	}

	fetched, err := handler.GetGoalByID(&goal.ID)
	if err != nil {
		t.Fatalf("GetGoalByID() error = %v", err)
	}
	if fetched == nil {
		t.Fatal("expected goal to exist")
	}
	if fetched.Description != nil {
		t.Fatalf("expected nil description, got %v", *fetched.Description)
	}
}

func TestGoalsRepositoryHandler_GetGoalByID_NotFound(t *testing.T) {
	pool := testPool(t)
	handler := NewGoalsRepositoryHandler(context.Background(), pool, nil)

	missingID := "00000000-0000-0000-0000-000000000000"
	goal, err := handler.GetGoalByID(&missingID)
	if err != nil {
		t.Fatalf("GetGoalByID() error = %v", err)
	}
	if goal != nil {
		t.Fatalf("expected nil goal, got %+v", goal)
	}
}

func TestGoalsRepositoryHandler_GetGoals_FiltersByProject(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	projectsHandler := NewProjectsRepositoryHandler(context.Background(), pool, nil)
	projectA := createTestProject(t, projectsHandler, organizationID)
	projectB := createTestProject(t, projectsHandler, organizationID)
	handler := NewGoalsRepositoryHandler(context.Background(), pool, nil)

	mustSaveGoal(t, handler, projectA.ID, "A1", sirkel_domain.GoalStateActive)
	mustSaveGoal(t, handler, projectA.ID, "A2", sirkel_domain.GoalStateActive)
	mustSaveGoal(t, handler, projectB.ID, "B1", sirkel_domain.GoalStateActive)

	goals, err := handler.GetGoals(&GetGoalsParams{ProjectID: &projectA.ID})
	if err != nil {
		t.Fatalf("GetGoals() error = %v", err)
	}
	if len(goals) != 2 {
		t.Fatalf("expected 2 goals for project A, got %d", len(goals))
	}
	for _, g := range goals {
		if g.ProjectID != projectA.ID {
			t.Fatalf("expected goal to belong to project %q, got %q", projectA.ID, g.ProjectID)
		}
	}
}

func TestGoalsRepositoryHandler_GetGoals_Pagination(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	handler := NewGoalsRepositoryHandler(context.Background(), pool, nil)

	for i := range 3 {
		mustSaveGoal(t, handler, project.ID, fmt.Sprintf("Goal %d", i), sirkel_domain.GoalStateActive)
	}

	limit := 2
	offset := 1
	goals, err := handler.GetGoals(&GetGoalsParams{
		ProjectID: &project.ID,
		Limit:     &limit,
		Offset:    &offset,
	})
	if err != nil {
		t.Fatalf("GetGoals() error = %v", err)
	}
	if len(goals) != 2 {
		t.Fatalf("expected 2 goals with limit/offset, got %d", len(goals))
	}
}

func TestGoalsRepositoryHandler_CountGoals(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	projectsHandler := NewProjectsRepositoryHandler(context.Background(), pool, nil)
	projectA := createTestProject(t, projectsHandler, organizationID)
	projectB := createTestProject(t, projectsHandler, organizationID)
	handler := NewGoalsRepositoryHandler(context.Background(), pool, nil)

	mustSaveGoal(t, handler, projectA.ID, "A1", sirkel_domain.GoalStateActive)
	mustSaveGoal(t, handler, projectA.ID, "A2", sirkel_domain.GoalStateActive)
	mustSaveGoal(t, handler, projectB.ID, "B1", sirkel_domain.GoalStateActive)

	count, err := handler.CountGoals(&CountGoalsParams{ProjectID: &projectA.ID})
	if err != nil {
		t.Fatalf("CountGoals() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	total, err := handler.CountGoals(&CountGoalsParams{})
	if err != nil {
		t.Fatalf("CountGoals() error = %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total count 3, got %d", total)
	}
}

func TestGoalsRepositoryHandler_DeleteGoal(t *testing.T) {
	pool := testPool(t)
	organizationID := createTestOrganization(t, pool)
	project := createTestProject(t, NewProjectsRepositoryHandler(context.Background(), pool, nil), organizationID)
	handler := NewGoalsRepositoryHandler(context.Background(), pool, nil)

	goal := mustSaveGoal(t, handler, project.ID, "To delete", sirkel_domain.GoalStateActive)

	if err := handler.DeleteGoal(&goal.ID); err != nil {
		t.Fatalf("DeleteGoal() error = %v", err)
	}

	fetched, err := handler.GetGoalByID(&goal.ID)
	if err != nil {
		t.Fatalf("GetGoalByID() error = %v", err)
	}
	if fetched != nil {
		t.Fatalf("expected goal to be deleted, got %+v", fetched)
	}
}
