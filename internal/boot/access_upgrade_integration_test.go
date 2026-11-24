package boot_test

import (
	"context"
	"os"
	"testing"

	driver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/NSObjects/PersonaOS/internal/boot"
	accessdomain "github.com/NSObjects/PersonaOS/internal/modules/access/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/infrastructure/mysqljson"
)

func accessIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("PERSONAOS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("PERSONAOS_TEST_MYSQL_DSN must be set for MySQL integration tests")
	}
	cfg, err := driver.ParseDSN(dsn)
	if err != nil || cfg.DBName == "" {
		t.Fatalf("invalid MySQL integration DSN: %v", err)
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

const agentDemoMenuPath = "/agent-demo"

type persistedRoleGrants struct {
	ID          int64
	Permissions mysqljson.Strings
	MenuIDs     mysqljson.Int64s
	APIIDs      mysqljson.Int64s
	ButtonIDs   mysqljson.Int64s
}

func containsPermission(values []string, token string) bool {
	for _, v := range values {
		if v == token {
			return true
		}
	}
	return false
}

func containsResourceID(values []int64, id int64) bool {
	for _, v := range values {
		if v == id {
			return true
		}
	}
	return false
}

// This test deliberately checks grant repair and repeatability on one existing database.
//
//nolint:gocyclo,funlen // Integration assertions intentionally cover every grant category.
func TestUpgradeExistingDatabaseRestoresD1Catalog(t *testing.T) {
	db := accessIntegrationDB(t)
	ctx := context.Background()
	var root persistedRoleGrants
	if err := db.Table("access_roles").
		Where("code = ?", accessdomain.RoleCodeSuperAdmin).Take(&root).Error; err != nil {
		t.Fatalf("CI must initialize existing database before upgrade test: %v", err)
	}
	menuPaths := []string{"/personaos", "/characters", "/device-types", "/capability-resources", agentDemoMenuPath}
	apiGroups := []string{"character", "device_type", "capability_resource"}
	var menuRows []struct {
		ID   int64
		Path string
	}
	if err := db.Table("access_menus").Select("id, path").
		Where("path IN ?", menuPaths).
		Find(&menuRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(menuRows) != 5 {
		t.Fatalf("PersonaOS menus = %d, want 5", len(menuRows))
	}
	var apiRows []struct {
		ID int64
	}
	if err := db.Table("access_apis").Select("id").
		Where("api_group IN ?", apiGroups).
		Find(&apiRows).Error; err != nil {
		t.Fatal(err)
	}
	apiCount := len(apiRows)
	if apiCount < 23 {
		t.Fatalf("PersonaOS API catalog incomplete: %d", len(apiRows))
	}
	omittedMenus := map[int64]struct{}{}
	omittedAPIs := map[int64]struct{}{}
	for _, row := range menuRows {
		omittedMenus[row.ID] = struct{}{}
	}
	for _, row := range apiRows {
		omittedAPIs[row.ID] = struct{}{}
	}
	legacyPermissions := make([]string, 0, len(root.Permissions))
	for _, token := range root.Permissions {
		if token != accessdomain.PermissionCharacterRead &&
			token != accessdomain.PermissionCharacterCreate &&
			token != accessdomain.PermissionDeviceTypeRead &&
			token != accessdomain.PermissionDeviceTypeCreate &&
			token != accessdomain.PermissionCapabilityResourceRead &&
			token != accessdomain.PermissionCapabilityResourceCreate {
			legacyPermissions = append(legacyPermissions, token)
		}
	}
	legacyMenus := make([]int64, 0, len(root.MenuIDs))
	for _, id := range root.MenuIDs {
		if _, omitted := omittedMenus[id]; !omitted {
			legacyMenus = append(legacyMenus, id)
		}
	}
	legacyAPIs := make([]int64, 0, len(root.APIIDs))
	for _, id := range root.APIIDs {
		if _, omitted := omittedAPIs[id]; !omitted {
			legacyAPIs = append(legacyAPIs, id)
		}
	}
	err := db.Table("access_roles").Where("id = ?", root.ID).
		Updates(map[string]any{
			"permissions": mysqljson.Strings(legacyPermissions),
			"menu_ids":    mysqljson.Int64s(legacyMenus),
			"api_ids":     mysqljson.Int64s(legacyAPIs),
		}).Error
	if err != nil {
		t.Fatal(err)
	}

	// Simulate an older installation without the dialogue menu or chat route.
	if err := db.Exec("DELETE FROM access_menus WHERE path = ?", agentDemoMenuPath).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM access_apis WHERE method = ? AND path = ?", "POST", "/api/demo/chat").Error; err != nil {
		t.Fatal(err)
	}

	// Execute the SAME operator-controlled upgrade function used by the CLI.
	if err := boot.UpgradeAccessAuthorization(ctx, "../../configs/config.ci.toml"); err != nil {
		t.Fatalf("upgrade existing authorization data: %v", err)
	}
	var restored persistedRoleGrants
	if err := db.Table("access_roles").Where("id = ?", root.ID).Take(&restored).Error; err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{
		accessdomain.PermissionCharacterRead,
		accessdomain.PermissionCharacterCreate,
		accessdomain.PermissionCharacterUpdate,
		accessdomain.PermissionCharacterDelete,
		accessdomain.PermissionDeviceTypeRead,
		accessdomain.PermissionDeviceTypeCreate,
		accessdomain.PermissionDeviceTypeUpdate,
		accessdomain.PermissionDeviceTypeDelete,
		accessdomain.PermissionCapabilityResourceRead,
		accessdomain.PermissionCapabilityResourceCreate,
		accessdomain.PermissionCapabilityResourceUpdate,
		accessdomain.PermissionCapabilityResourceDelete,
	} {
		if !containsPermission(restored.Permissions, token) {
			t.Errorf("root role missing D1 permission %q", token)
		}
	}
	if err := db.Table("access_menus").Select("id, path").
		Where("path IN ?", menuPaths).
		Find(&menuRows).Error; err != nil || len(menuRows) != 5 {
		t.Fatalf("restore PersonaOS menus: %v, rows=%v", err, menuRows)
	}
	if err := db.Table("access_apis").Select("id").
		Where("api_group IN ?", apiGroups).
		Find(&apiRows).Error; err != nil || len(apiRows) != apiCount {
		t.Fatalf("restore PersonaOS APIs: %v, rows=%v", err, apiRows)
	}
	for _, row := range menuRows {
		if !containsResourceID(restored.MenuIDs, row.ID) {
			t.Errorf("root role missing menu %q", row.Path)
		}
	}
	for _, row := range apiRows {
		if !containsResourceID(restored.APIIDs, row.ID) {
			t.Errorf("root role missing D1 API ID %d", row.ID)
		}
	}

	var buttonIDs []int64
	if err := db.Table("access_menu_buttons").Where("menu_id IN ?", restored.MenuIDs).Pluck("id", &buttonIDs).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range buttonIDs {
		if !containsResourceID(restored.ButtonIDs, id) {
			t.Errorf("root role missing button %d", id)
		}
	}

	var chatRoute struct{ ID int64 }
	if err := db.Table("access_apis").Select("id").Where("method = ? AND path = ?", "POST", "/api/demo/chat").Take(&chatRoute).Error; err != nil || !containsResourceID(restored.APIIDs, chatRoute.ID) {
		t.Fatalf("restored chat route grant: %v, id=%d", err, chatRoute.ID)
	}

	// Upgrade must be repeatable without duplicating grants.
	if err := boot.UpgradeAccessAuthorization(ctx, "../../configs/config.ci.toml"); err != nil {
		t.Fatalf("second authorization upgrade: %v", err)
	}
	var repeated persistedRoleGrants
	if err := db.Table("access_roles").Where("id = ?", root.ID).Take(&repeated).Error; err != nil {
		t.Fatal(err)
	}
	if len(repeated.Permissions) != len(restored.Permissions) ||
		len(repeated.MenuIDs) != len(restored.MenuIDs) ||
		len(repeated.APIIDs) != len(restored.APIIDs) ||
		len(repeated.ButtonIDs) != len(restored.ButtonIDs) {
		t.Errorf("upgrade changed grant counts on second run")
	}
}
