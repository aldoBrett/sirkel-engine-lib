package sirkel_auth

import (
	"testing"

	"sirkel-engine-lib/sirkel_errors"
)

func TestSirkelAuthHandler_OrganizationsIndex_Basic(t *testing.T) {
	pool := testPool(t)
	insertTestOrganization(t, pool, "Acme")
	insertTestOrganization(t, pool, "Beta Corp")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	resp, err := h.OrganizationsIndex(OrganizationsIndexParams{})
	if err != nil {
		t.Fatalf("OrganizationsIndex() error = %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("expected total 2, got %d", resp.Total)
	}
	if len(resp.Organizations) != 2 {
		t.Fatalf("expected 2 organizations, got %d", len(resp.Organizations))
	}
	if resp.Limit != defaultOrganizationsIndexLimit {
		t.Fatalf("expected default limit %d, got %d", defaultOrganizationsIndexLimit, resp.Limit)
	}
	if resp.Offset != 0 {
		t.Fatalf("expected offset 0, got %d", resp.Offset)
	}
}

func TestSirkelAuthHandler_OrganizationsIndex_TextSearch(t *testing.T) {
	pool := testPool(t)
	insertTestOrganization(t, pool, "Acme Rockets")
	insertTestOrganization(t, pool, "Beta Corp")

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	term := "rocket"
	resp, err := h.OrganizationsIndex(OrganizationsIndexParams{TextSearch: &term})
	if err != nil {
		t.Fatalf("OrganizationsIndex() error = %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("expected total 1, got %d", resp.Total)
	}
	if len(resp.Organizations) != 1 || resp.Organizations[0].Name != "Acme Rockets" {
		t.Fatalf("expected only Acme Rockets, got %+v", resp.Organizations)
	}
}

func TestSirkelAuthHandler_OrganizationsIndex_Pagination(t *testing.T) {
	pool := testPool(t)
	for i := 0; i < 3; i++ {
		insertTestOrganization(t, pool, "Org")
	}

	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})
	limit := 2
	offset := 1
	resp, err := h.OrganizationsIndex(OrganizationsIndexParams{Limit: &limit, Offset: &offset})
	if err != nil {
		t.Fatalf("OrganizationsIndex() error = %v", err)
	}
	if resp.Total != 3 {
		t.Fatalf("expected total 3, got %d", resp.Total)
	}
	if len(resp.Organizations) != 2 {
		t.Fatalf("expected 2 organizations with limit/offset, got %d", len(resp.Organizations))
	}
}

func TestSirkelAuthHandler_OrganizationsIndex_LimitCappedAtMax(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	limit := maxOrganizationsIndexLimit + 50
	resp, err := h.OrganizationsIndex(OrganizationsIndexParams{Limit: &limit})
	if err != nil {
		t.Fatalf("OrganizationsIndex() error = %v", err)
	}
	if resp.Limit != maxOrganizationsIndexLimit {
		t.Fatalf("expected limit capped at %d, got %d", maxOrganizationsIndexLimit, resp.Limit)
	}
}

func TestSirkelAuthHandler_OrganizationsIndex_InvalidLimit(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	limit := 0
	_, err := h.OrganizationsIndex(OrganizationsIndexParams{Limit: &limit})
	assertErrorCode(t, err, sirkel_errors.CodeInvalidLimit)
}

func TestSirkelAuthHandler_OrganizationsIndex_InvalidOffset(t *testing.T) {
	pool := testPool(t)
	h := NewSirkelAuthHandler(SirkelAuthHandlerParams{Pool: pool})

	offset := -1
	_, err := h.OrganizationsIndex(OrganizationsIndexParams{Offset: &offset})
	assertErrorCode(t, err, sirkel_errors.CodeInvalidOffset)
}
