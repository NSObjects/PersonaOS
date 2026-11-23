package usecase_test

import (
	"context"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/adapters/memory"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

func requireCode(t *testing.T, err error, code int) {
	t.Helper()
	appErr, ok := apperr.Parse(err)
	if !ok || appErr.Code() != code {
		t.Fatalf("error=%v, want application code %d", err, code)
	}
}

func TestDeviceTypesReuseCharacterAndProtectReferences(t *testing.T) {
	ctx := context.Background()
	uc := usecase.New(memory.New())
	c, err := uc.CreateCharacter(ctx, usecase.CharacterInput{Code: "shinchan", Name: "蜡笔小新", Persona: "好奇、活泼"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		code   string
		camera bool
	}{{"A3", false}, {"A4", true}} {
		got, err := uc.CreateDeviceType(ctx, usecase.DeviceTypeInput{Code: item.code, Name: item.code, CharacterID: c.ID,
			HasCamera: item.camera, HasMicrophone: true, HasSpeaker: true})
		if err != nil {
			t.Fatalf("CreateDeviceType(%s) error = %v", item.code, err)
		}
		if got.CharacterID != c.ID || got.HasCamera != item.camera {
			t.Fatalf("CreateDeviceType(%s) = %+v", item.code, got)
		}
	}
	types, err := uc.ListDeviceTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 || types[0].CharacterID != types[1].CharacterID {
		t.Fatalf("types=%+v, want two sharing one character", types)
	}
	requireCode(t, uc.DeleteCharacter(ctx, c.ID), apperr.ErrConflict)
	updated, err := uc.UpdateCharacter(ctx, c.ID, usecase.CharacterInput{Name: "小新", Persona: "顽皮、热情"})
	if err != nil || updated.Name != "小新" {
		t.Fatalf("UpdateCharacter()=(%+v,%v), want 小新", updated, err)
	}
	for _, d := range types {
		if err := uc.DeleteDeviceType(ctx, d.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := uc.DeleteCharacter(ctx, c.ID); err != nil {
		t.Fatalf("DeleteCharacter() error = %v", err)
	}
}
func TestCatalogRejectsInvalidRelationsAndCodes(t *testing.T) {
	ctx := context.Background()
	uc := usecase.New(memory.New())
	_, err := uc.CreateCharacter(ctx, usecase.CharacterInput{Code: "!", Name: "bad", Persona: "x"})
	requireCode(t, err, apperr.ErrBadRequest)
	_, err = uc.CreateDeviceType(ctx, usecase.DeviceTypeInput{Code: "A3", Name: "A3", CharacterID: 123})
	requireCode(t, err, apperr.ErrNotFound)
	_, err = uc.CreateCharacter(ctx, usecase.CharacterInput{Code: "shinchan", Name: "角色", Persona: "性格"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = uc.CreateCharacter(ctx, usecase.CharacterInput{Code: "SHINCHAN", Name: "重复", Persona: "性格"})
	requireCode(t, err, apperr.ErrConflict)
}
func TestCanceledContextStopsCatalogWrites(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	uc := usecase.New(memory.New())
	_, err := uc.CreateCharacter(ctx, usecase.CharacterInput{Code: "foo", Name: "foo", Persona: "foo"})
	if err != context.Canceled {
		t.Fatalf("CreateCharacter() error = %v, want context.Canceled", err)
	}
}
