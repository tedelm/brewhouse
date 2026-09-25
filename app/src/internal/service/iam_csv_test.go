package service_test

import (
	"errors"
	"strings"
	"testing"

	"brewhouse/internal/service"
)

func TestUsersCSV_ExportImportUpsertSkipsPassword(t *testing.T) {
	_, users, _, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	csv1 := "" +
		"username,password,email,role,active,first_name,last_name,address_line1,address_line2,phone,instagram\n" +
		"alice,secret1,alice@brew.test,user,1,Alice,A,,,,,\n"
	res, err := users.ImportUsersCSV(admin, []byte(csv1))
	if err != nil {
		t.Fatalf("import create: %v", err)
	}
	if res.Created != 1 || res.Updated != 0 || res.Failed != 0 {
		t.Fatalf("unexpected create result: %+v", res)
	}
	if _, err := users.Authenticate("alice", "secret1"); err != nil {
		t.Fatalf("auth after create: %v", err)
	}

	csv2 := "" +
		"username,password,email,role,active,first_name,last_name,address_line1,address_line2,phone,instagram\n" +
		"alice,changed-password,alice2@brew.test,superuser,1,Alice,Updated,,,,,\n"
	res, err = users.ImportUsersCSV(admin, []byte(csv2))
	if err != nil {
		t.Fatalf("import update: %v", err)
	}
	if res.Created != 0 || res.Updated != 1 || res.Failed != 0 {
		t.Fatalf("unexpected update result: %+v", res)
	}
	if _, err := users.Authenticate("alice", "secret1"); err != nil {
		t.Fatalf("old password should still work: %v", err)
	}
	if _, err := users.Authenticate("alice", "changed-password"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("csv password on update should be ignored, got %v", err)
	}
	u, err := users.Authenticate("alice", "secret1")
	if err != nil {
		t.Fatalf("reauth: %v", err)
	}
	if u.Email != "alice2@brew.test" || u.Role != service.RoleSuperuser || u.LastName != "Updated" {
		t.Fatalf("fields not updated: %+v", u)
	}

	exported, err := users.ExportUsersCSV()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	text := string(exported)
	if !strings.Contains(text, "username,password,email,role,active") {
		t.Fatalf("missing header: %s", text)
	}
	if !strings.Contains(text, "alice") {
		t.Fatalf("missing alice in export")
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "alice,") {
			cols := strings.Split(line, ",")
			if len(cols) < 2 || cols[1] != "" {
				t.Fatalf("export password column should be empty, got %q", line)
			}
		}
	}

	nonAdmin := service.Actor{UserID: u.ID, Role: service.RoleUser}
	if _, err := users.ImportUsersCSV(nonAdmin, []byte(csv1)); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestBreweriesCSV_ExportImportUpsert(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	csv1 := "name,contact_name,contact_email,contact_phone,instagram\n" +
		"Alpha Brew,Ann,ann@a.test,111,https://instagram.com/alpha\n"
	res, err := breweries.ImportBreweriesCSV(admin, []byte(csv1))
	if err != nil {
		t.Fatalf("import create: %v", err)
	}
	if res.Created != 1 || res.Updated != 0 {
		t.Fatalf("unexpected create: %+v", res)
	}

	csv2 := "name,contact_name,contact_email,contact_phone,instagram\n" +
		"Alpha Brew,Bob,bob@a.test,222,https://instagram.com/bob\n" +
		"Beta Brew,Bea,bea@b.test,333,\n"
	res, err = breweries.ImportBreweriesCSV(admin, []byte(csv2))
	if err != nil {
		t.Fatalf("import upsert: %v", err)
	}
	if res.Created != 1 || res.Updated != 1 || res.Failed != 0 {
		t.Fatalf("unexpected upsert: %+v", res)
	}

	list, err := breweries.List(admin)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var alpha, beta *service.Brewery
	for i := range list {
		switch list[i].Name {
		case "Alpha Brew":
			alpha = &list[i]
		case "Beta Brew":
			beta = &list[i]
		}
	}
	if alpha == nil || alpha.ContactName != "Bob" || alpha.ContactEmail != "bob@a.test" || alpha.ContactPhone != "222" || alpha.Instagram != "https://instagram.com/bob" {
		t.Fatalf("alpha not updated: %+v", alpha)
	}
	if beta == nil || beta.ContactName != "Bea" {
		t.Fatalf("beta not created: %+v", beta)
	}

	exported, err := breweries.ExportBreweriesCSV(admin)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	text := string(exported)
	if !strings.Contains(text, "name,contact_name,contact_email,contact_phone,instagram") {
		t.Fatalf("missing header")
	}
	if !strings.Contains(text, "Alpha Brew") || !strings.Contains(text, "Beta Brew") {
		t.Fatalf("missing rows: %s", text)
	}

	nonAdmin := service.Actor{UserID: 99, Role: service.RoleUser}
	if _, err := breweries.ImportBreweriesCSV(nonAdmin, []byte(csv1)); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestMembersCSV_ExportImportAddUpdateNoDelete(t *testing.T) {
	_, users, breweries, _, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	brewery, err := breweries.Create(admin, "Gamma Brew", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("create brewery: %v", err)
	}
	alice, err := users.Create("alice", "secret", "alice@brew.test", service.RoleUser, service.UserContact{})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := users.Create("bob", "secret", "bob@brew.test", service.RoleUser, service.UserContact{})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	if err := breweries.AddMember(admin, brewery.ID, alice.ID, service.RoleUser); err != nil {
		t.Fatalf("add alice: %v", err)
	}
	if err := breweries.AddMember(admin, brewery.ID, bob.ID, service.RoleUser); err != nil {
		t.Fatalf("add bob: %v", err)
	}

	exported, err := breweries.ExportMembersCSV(admin)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	text := string(exported)
	if !strings.Contains(text, "brewery_name,username,role") {
		t.Fatalf("missing header: %s", text)
	}
	if !strings.Contains(text, "Gamma Brew,alice,user") || !strings.Contains(text, "Gamma Brew,bob,user") {
		t.Fatalf("missing membership rows: %s", text)
	}

	csvUpdate := "brewery_name,username,role\n" +
		"Gamma Brew,alice,brewery_admin\n"
	res, err := breweries.ImportMembersCSV(admin, []byte(csvUpdate))
	if err != nil {
		t.Fatalf("import role update: %v", err)
	}
	if res.Created != 0 || res.Updated != 1 || res.Failed != 0 {
		t.Fatalf("unexpected update result: %+v", res)
	}

	members, err := breweries.ListMembers(admin, brewery.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("import must not remove omitted members, got %d", len(members))
	}
	var aliceRole, bobRole string
	for _, m := range members {
		switch m.UserID {
		case alice.ID:
			aliceRole = m.Role
		case bob.ID:
			bobRole = m.Role
		}
	}
	if aliceRole != service.RoleBreweryAdmin {
		t.Fatalf("alice role want brewery_admin got %q", aliceRole)
	}
	if bobRole != service.RoleUser {
		t.Fatalf("bob should remain user, got %q", bobRole)
	}

	cara, err := users.Create("cara", "secret", "cara@brew.test", service.RoleUser, service.UserContact{})
	if err != nil {
		t.Fatalf("create cara: %v", err)
	}
	csvAdd := "brewery_name,username,role\n" +
		"Gamma Brew,cara,superuser\n"
	res, err = breweries.ImportMembersCSV(admin, []byte(csvAdd))
	if err != nil {
		t.Fatalf("import add: %v", err)
	}
	if res.Created != 1 || res.Updated != 0 || res.Failed != 0 {
		t.Fatalf("unexpected create result: %+v", res)
	}
	members, err = breweries.ListMembers(admin, brewery.ID)
	if err != nil {
		t.Fatalf("list after add: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("want 3 members after add, got %d", len(members))
	}
	foundCara := false
	for _, m := range members {
		if m.UserID == cara.ID && m.Role == service.RoleSuperuser {
			foundCara = true
		}
	}
	if !foundCara {
		t.Fatalf("cara not added as superuser")
	}

	csvBad := "brewery_name,username,role\n" +
		"NoSuch Brew,alice,user\n" +
		"Gamma Brew,nobody,user\n"
	res, err = breweries.ImportMembersCSV(admin, []byte(csvBad))
	if err != nil {
		t.Fatalf("import bad rows: %v", err)
	}
	if res.Failed != 2 || len(res.Errors) < 2 {
		t.Fatalf("want 2 failed rows, got %+v", res)
	}

	nonAdmin := service.Actor{UserID: alice.ID, Role: service.RoleUser}
	if _, err := breweries.ExportMembersCSV(nonAdmin); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("export expected forbidden, got %v", err)
	}
	if _, err := breweries.ImportMembersCSV(nonAdmin, []byte(csvAdd)); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("import expected forbidden, got %v", err)
	}
}
