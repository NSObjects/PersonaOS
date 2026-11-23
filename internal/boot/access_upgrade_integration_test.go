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

type persistedRoleGrants struct {
	ID          int64
	Permissions mysqljson.Strings
	MenuIDs     mysqljson.Int64s
	APIIDs      mysqljson.Int64s
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

func TestUpgradeExistingDatabaseRestoresD1Catalog(t *testing.T) {
	db := accessIntegrationDB(t)
	ctx := context.Background()
	var root persistedRoleGrants
	if err := db.Table("access_roles").
		Where("code = ?", accessdomain.RoleCodeSuperAdmin).Take(&root).Error; err != nil {
		t.Fatalf("CI must initialize existing database before upgrade test: %v", err)
	}
	var menuRows []struct {
		ID   int64
		Path string
	}
	if err := db.Table("access_menus").Select("id, path").
		Where("path IN ?", []string{"/personaos", "/characters", "/device-types"}).
		Find(&menuRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(menuRows) != 3 {
		t.Fatalf("D1 menus = %d, want 3", len(menuRows))
	}
	var apiRows []struct {
		ID int64
	}
	if err := db.Table("access_apis").Select("id").
		Where("path IN ?", []string{"/api/characters", "/api/device-types"}).
		Find(&apiRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(apiRows) != 4 {
		t.Fatalf("D1 list/create APIs = %d, want 4", len(apiRows))
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
			token != accessdomain.PermissionDeviceTypeCreate {
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
	} {
		if !containsPermission(restored.Permissions, token) {
			t.Errorf("root role missing D1 permission %q", token)
		}
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
		len(repeated.APIIDs) != len(restored.APIIDs) {
		t.Errorf("upgrade changed grant counts on second run")
	}
}
