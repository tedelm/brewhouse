package service_test

import (
	"strings"
	"testing"

	"brewhouse/internal/service"
)

func TestUser_CreateWithEmailAndBreweryMembership(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	brewery, err := breweries.Create(admin, "Member Brew", "C", "c@t.com", "", nil)
	if err != nil {
		t.Fatalf("brewery: %v", err)
	}

	created, err := users.Create("alice", "secret", "alice@brew.test", service.RoleUser, service.UserContact{
		FirstName:    "Alice",
		LastName:     "Brewer",
		AddressLine1: "Street 1",
		AddressLine2: "Apt 2",
		Phone:        "+46701234567",
		Instagram:    "https://instagram.com/alice",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Email != "alice@brew.test" {
		t.Fatalf("email=%q", created.Email)
	}
	if created.FirstName != "Alice" || created.LastName != "Brewer" {
		t.Fatalf("name=%q %q", created.FirstName, created.LastName)
	}
	if created.AddressLine1 != "Street 1" || created.AddressLine2 != "Apt 2" {
		t.Fatalf("address=%q %q", created.AddressLine1, created.AddressLine2)
	}
	if created.Phone != "+46701234567" || created.Instagram != "https://instagram.com/alice" {
		t.Fatalf("phone/instagram=%q %q", created.Phone, created.Instagram)
	}
	if created.Role != service.RoleUser {
		t.Fatalf("role=%q", created.Role)
	}

	got, err := users.Get(created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Instagram != "https://instagram.com/alice" {
		t.Fatalf("get instagram=%q", got.Instagram)
	}

	memberRole := service.MembershipRoleForCreate(created.Role)
	if err := breweries.AddMember(admin, brewery.ID, created.ID, memberRole); err != nil {
		t.Fatalf("add member: %v", err)
	}
	members, err := breweries.ListMembers(admin, brewery.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	found := false
	for _, m := range members {
		if m.UserID == created.ID && m.Role == service.RoleUser {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected user membership, got %#v", members)
	}
}

func TestUser_CreateAdminWithoutBrewery(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	brewery, err := breweries.Create(admin, "Other Brew", "", "", "", nil)
	if err != nil {
		t.Fatalf("brewery: %v", err)
	}

	created, err := users.Create("boss", "secret", "boss@brew.test", service.RoleAdmin, service.UserContact{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Role != service.RoleAdmin {
		t.Fatalf("role=%q", created.Role)
	}

	members, err := breweries.ListMembers(admin, brewery.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, m := range members {
		if m.UserID == created.ID {
			t.Fatalf("admin without brewery attach should have no membership")
		}
	}
}

func TestMembershipRoleForCreate(t *testing.T) {
	if got := service.MembershipRoleForCreate(service.RoleAdmin); got != service.RoleBreweryAdmin {
		t.Fatalf("admin -> %q", got)
	}
	if got := service.MembershipRoleForCreate(service.RoleUser); got != service.RoleUser {
		t.Fatalf("user -> %q", got)
	}
	if got := service.MembershipRoleForCreate(service.RoleSuperuser); got != service.RoleSuperuser {
		t.Fatalf("superuser -> %q", got)
	}
}

func TestUser_CreateRequiresEmailAndValidRole(t *testing.T) {
	_, users, _, _, _, _, _ := testDB(t)
	if _, err := users.Create("x", "y", "", service.RoleUser, service.UserContact{}); err == nil {
		t.Fatal("expected email required")
	}
	if _, err := users.Create("x", "y", "x@y.z", "brewery_admin", service.UserContact{}); err == nil {
		t.Fatal("expected invalid role")
	}
}

func TestUser_CreateRejectsInvalidInstagram(t *testing.T) {
	_, users, _, _, _, _, _ := testDB(t)
	invalid := []string{"@handle", "instagram.com/x", "ftp://instagram.com/x", "not a url"}
	for _, ig := range invalid {
		if _, err := users.Create("iguser", "secret", "ig@brew.test", service.RoleUser, service.UserContact{Instagram: ig}); err == nil {
			t.Fatalf("expected invalid instagram %q", ig)
		} else if !strings.Contains(err.Error(), "instagram") {
			t.Fatalf("expected instagram error for %q, got %v", ig, err)
		}
	}
	if _, err := users.Create("igok", "secret", "igok@brew.test", service.RoleUser, service.UserContact{
		Instagram: "https://www.instagram.com/brewhouse",
	}); err != nil {
		t.Fatalf("valid instagram: %v", err)
	}
}

func TestUser_UpdateProfile(t *testing.T) {
	_, users, _, _, _, _, _ := testDB(t)
	created, err := users.Create("cara", "oldpass", "cara@old.test", service.RoleUser, service.UserContact{
		FirstName: "Cara",
		Phone:     "111",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := users.UpdateProfile(created.ID, "cara@new.test", "newpass", service.UserContact{
		FirstName:    "Caroline",
		LastName:     "Lager",
		AddressLine1: "Brew St 3",
		Phone:        "222",
		Instagram:    "http://instagram.com/cara",
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.Email != "cara@new.test" {
		t.Fatalf("email=%q", updated.Email)
	}
	if updated.FirstName != "Caroline" || updated.LastName != "Lager" || updated.Instagram != "http://instagram.com/cara" {
		t.Fatalf("contact=%#v", updated)
	}
	if _, err := users.Authenticate("cara", "oldpass"); err != service.ErrInvalidCredentials {
		t.Fatalf("old password should fail, got %v", err)
	}
	if _, err := users.Authenticate("cara", "newpass"); err != nil {
		t.Fatalf("new password auth: %v", err)
	}
	emailOnly, err := users.UpdateProfile(created.ID, "cara@keep.test", "", service.UserContact{
		FirstName: "Caroline",
		LastName:  "Lager",
		Instagram: "http://instagram.com/cara",
	})
	if err != nil {
		t.Fatalf("email only: %v", err)
	}
	if emailOnly.Email != "cara@keep.test" {
		t.Fatalf("email=%q", emailOnly.Email)
	}
	if emailOnly.Phone != "" {
		t.Fatalf("phone should be cleared, got %q", emailOnly.Phone)
	}
	if _, err := users.Authenticate("cara", "newpass"); err != nil {
		t.Fatalf("password should be unchanged: %v", err)
	}
	if _, err := users.UpdateProfile(created.ID, "cara@keep.test", "", service.UserContact{Instagram: "@bad"}); err == nil {
		t.Fatal("expected invalid instagram on profile update")
	}
}

func TestUser_UpdateContact(t *testing.T) {
	_, users, _, _, _, _, _ := testDB(t)
	created, err := users.Create("dave", "secret", "dave@brew.test", service.RoleUser, service.UserContact{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := users.Update(created.ID, "dave", "", "dave@brew.test", service.RoleUser, service.UserContact{
		FirstName: "Dave",
		LastName:  "Malt",
		Phone:     "333",
		Instagram: "https://instagram.com/dave",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.FirstName != "Dave" || updated.Instagram != "https://instagram.com/dave" {
		t.Fatalf("updated=%#v", updated)
	}
	if _, err := users.Update(created.ID, "dave", "", "dave@brew.test", service.RoleUser, service.UserContact{
		Instagram: "not-a-url",
	}); err == nil {
		t.Fatal("expected invalid instagram on update")
	}
}

func TestUser_SetActiveBlocksLogin(t *testing.T) {
	_, users, _, _, _, _, _ := testDB(t)
	created, err := users.Create("bob", "secret", "bob@brew.test", service.RoleUser, service.UserContact{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !created.Active {
		t.Fatal("expected new user active")
	}

	updated, err := users.SetActive(created.ID, false)
	if err != nil {
		t.Fatalf("set active: %v", err)
	}
	if updated.Active {
		t.Fatal("expected inactive")
	}

	list, err := users.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, u := range list {
		if u.ID == created.ID {
			found = true
			if u.Active {
				t.Fatal("list should show inactive")
			}
		}
	}
	if !found {
		t.Fatal("user missing from list")
	}

	if _, err := users.Authenticate("bob", "secret"); err != service.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	if _, err := users.SetActive(created.ID, true); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if _, err := users.Authenticate("bob", "secret"); err != nil {
		t.Fatalf("auth after reactivate: %v", err)
	}
}
