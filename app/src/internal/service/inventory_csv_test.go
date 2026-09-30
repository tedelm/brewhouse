package service_test

import (
	"errors"
	"strings"
	"testing"

	"brewhouse/internal/service"
)

const hopsCSVHeader = "category,name,unit,qty,cost_price,producer,supplier,item_type,link"
const maltCSVHeader = "category,name,unit,qty,cost_price,producer,supplier,item_type,min_ebc,max_ebc,link"
const yeastCSVHeader = "category,name,unit,qty,cost_price,producer,supplier,item_type,link,pitch_min_g_hl,pitch_max_g_hl,pack_size_g,temp_min_c,temp_max_c"

func TestInventoryCSV_ExportImportUpsert(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	csv1 := hopsCSVHeader + "\n" +
		"hops,Cascade,g,100,12.5,Yakima,,T90,https://example.com/cascade\n"
	res, err := inventory.ImportInventoryCSV(admin, service.CategoryHops, []byte(csv1))
	if err != nil {
		t.Fatalf("import create: %v", err)
	}
	if res.Created != 1 || res.Updated != 0 || res.Failed != 0 {
		t.Fatalf("unexpected create result: %+v", res)
	}

	csv2 := hopsCSVHeader + "\n" +
		"hops,Cascade,g,250,15,Yakima,,T90,https://example.com/cascade2\n" +
		"hops,Citra,g,50,20,Yakima,,T90,\n"
	res, err = inventory.ImportInventoryCSV(admin, service.CategoryHops, []byte(csv2))
	if err != nil {
		t.Fatalf("import upsert: %v", err)
	}
	if res.Created != 1 || res.Updated != 1 || res.Failed != 0 {
		t.Fatalf("unexpected upsert: %+v", res)
	}

	items, err := inventory.ListByCategory(service.CategoryHops)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var cascade, citra *service.InventoryItem
	for i := range items {
		switch items[i].Name {
		case "Cascade":
			cascade = &items[i]
		case "Citra":
			citra = &items[i]
		}
	}
	if cascade == nil || cascade.Qty != 250 || cascade.CostPrice != 15 || cascade.Link != "https://example.com/cascade2" {
		t.Fatalf("cascade not updated: %+v", cascade)
	}
	if citra == nil || citra.Qty != 50 || citra.Producer != "Yakima" {
		t.Fatalf("citra not created: %+v", citra)
	}

	exported, err := inventory.ExportInventoryCSV(service.CategoryHops)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	text := string(exported)
	if !strings.Contains(text, hopsCSVHeader) {
		t.Fatalf("missing header: %s", text)
	}
	if strings.Contains(text, "min_ebc") || strings.Contains(text, "pitch_min_g_hl") {
		t.Fatalf("hops export should omit malt/yeast columns: %s", text)
	}
	if !strings.Contains(text, "Cascade") || !strings.Contains(text, "Citra") {
		t.Fatalf("missing rows: %s", text)
	}

	nonAdmin := service.Actor{UserID: 99, Role: service.RoleUser}
	if _, err := inventory.ImportInventoryCSV(nonAdmin, service.CategoryHops, []byte(csv1)); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestInventoryCSV_CategoryMismatch(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	csv := hopsCSVHeader + "\n" +
		"yeast,Wrong Cat,pack,1,1,,,,\n"
	res, err := inventory.ImportInventoryCSV(admin, service.CategoryHops, []byte(csv))
	if err != nil {
		t.Fatalf("import should succeed with row errors: %v", err)
	}
	if res.Failed != 1 || res.Created != 0 || res.Updated != 0 {
		t.Fatalf("expected one failed row, got %+v", res)
	}
	if len(res.Errors) == 0 || !strings.Contains(res.Errors[0], "does not match") {
		t.Fatalf("expected category mismatch error, got %+v", res.Errors)
	}
}

func TestInventoryCSV_ExportCategoryColumns(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	if _, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryMalt, Name: "Pale", Unit: "kg", Qty: 1, MinEBC: 2, MaxEBC: 4,
	}); err != nil {
		t.Fatalf("create malt: %v", err)
	}
	if _, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryYeast, Name: "US-05", Unit: "pack", Qty: 1,
		PitchMinGHl: 50, PitchMaxGHl: 80, PackSizeG: 11.5, TempMinC: 18, TempMaxC: 26,
	}); err != nil {
		t.Fatalf("create yeast: %v", err)
	}

	maltCSV, err := inventory.ExportInventoryCSV(service.CategoryMalt)
	if err != nil {
		t.Fatalf("export malt: %v", err)
	}
	maltText := string(maltCSV)
	if !strings.HasPrefix(maltText, maltCSVHeader+"\n") {
		t.Fatalf("malt header mismatch: %s", maltText)
	}
	if strings.Contains(maltText, "pitch_min_g_hl") {
		t.Fatalf("malt export should omit yeast columns: %s", maltText)
	}

	yeastCSV, err := inventory.ExportInventoryCSV(service.CategoryYeast)
	if err != nil {
		t.Fatalf("export yeast: %v", err)
	}
	yeastText := string(yeastCSV)
	if !strings.HasPrefix(yeastText, yeastCSVHeader+"\n") {
		t.Fatalf("yeast header mismatch: %s", yeastText)
	}
	if strings.Contains(yeastText, "min_ebc") {
		t.Fatalf("yeast export should omit malt columns: %s", yeastText)
	}
}

func TestInventoryCSV_ExportInvalidCategory(t *testing.T) {
	_, _, _, inventory, _, _, _ := testDB(t)
	if _, err := inventory.ExportInventoryCSV("nope"); err == nil {
		t.Fatal("expected invalid category error")
	}
}

func TestExportOrderCSV(t *testing.T) {
	_, users, _, inventory, _, _, _ := testDB(t)
	_, admin := ensureAdminUser(t, users)

	item, err := inventory.Create(admin, service.InventoryItem{
		Category: service.CategoryMalt, Name: "Export Malt", Unit: "kg", Qty: 50, CostPrice: 12,
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	order, err := inventory.CreateOrder(admin, "export", "", []service.OrderLineInput{
		{InventoryItemID: item.ID, Qty: 5},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	data, err := inventory.ExportOrderCSV(order.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "item_name,category,qty,ordered_qty,unit,cost_price,line_cost,brewery_name,link") {
		t.Fatalf("missing header: %s", text)
	}
	if !strings.Contains(text, "Export Malt") {
		t.Fatalf("missing item name: %s", text)
	}
	if !strings.Contains(text, "12") || !strings.Contains(text, "60") {
		t.Fatalf("missing cost columns: %s", text)
	}

	if _, err := inventory.ExportOrderCSV(order.ID + 999); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
