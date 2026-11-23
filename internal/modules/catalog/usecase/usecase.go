// Package usecase coordinates the character catalog and device-type workflows.
package usecase

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
    "github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// Store persists the catalog and enforces unique codes.
type Store interface {
    ListCharacters(context.Context) ([]domain.Character, error)
    FindCharacter(context.Context, int64) (domain.Character, error)
    CreateCharacter(context.Context, domain.Character) (domain.Character, error)
    UpdateCharacter(context.Context, domain.Character) (domain.Character, error)
    DeleteCharacter(context.Context, int64) error
    ListDeviceTypes(context.Context) ([]domain.DeviceType, error)
    FindDeviceType(context.Context, int64) (domain.DeviceType, error)
    CreateDeviceType(context.Context, domain.DeviceType) (domain.DeviceType, error)
    UpdateDeviceType(context.Context, domain.DeviceType) (domain.DeviceType, error)
    DeleteDeviceType(context.Context, int64) error
}

// Usecase operates on shared characters and independently configured products.
type Usecase struct { store Store }

// New creates a catalog usecase.
func New(store Store) *Usecase { return &Usecase{store:store} }

// CharacterInput specifies an editable character. Code is immutable after creation.
type CharacterInput struct {
    Code string `json:"code"`
    Name string `json:"name"`
    Persona string `json:"persona"`
    Description string `json:"description"`
}

// DeviceTypeInput specifies an editable product model.
type DeviceTypeInput struct {
    Code string `json:"code"`
    Name string `json:"name"`
    Description string `json:"description"`
    CharacterID int64 `json:"character_id"`
    HasCamera bool `json:"has_camera"`
    HasMicrophone bool `json:"has_microphone"`
    HasSpeaker bool `json:"has_speaker"`
}

func (u *Usecase) ready(ctx context.Context) error {
    if ctx == nil { return apperr.NewBadRequest("context is required") }
    if err := ctx.Err(); err != nil { return err }
    if u == nil || u.store == nil { return apperr.New(apperr.ErrInternalServer,"catalog store is not configured") }
    return nil
}

func badDomain(err error) error {
    if errors.Is(err,domain.ErrInvalidCharacter) || errors.Is(err,domain.ErrInvalidDeviceType) {
        return apperr.WrapBadRequest(err,"invalid catalog input")
    }
    return err
}

// ListCharacters lists all characters for the D1 management UI.
func (u *Usecase) ListCharacters(ctx context.Context) ([]domain.Character,error) {
    if err:=u.ready(ctx);err!=nil{return nil,err}
    return u.store.ListCharacters(ctx)
}

// FindCharacter returns one character.
func (u *Usecase) FindCharacter(ctx context.Context,id int64)(domain.Character,error){
    if err:=u.ready(ctx);err!=nil{return domain.Character{},err}
    if id<=0{return domain.Character{},apperr.NewBadRequest("invalid character id")}
    return u.store.FindCharacter(ctx,id)
}

// CreateCharacter adds a reusable persona.
func (u *Usecase) CreateCharacter(ctx context.Context,in CharacterInput)(domain.Character,error){
    if err:=u.ready(ctx);err!=nil{return domain.Character{},err}
    value,err:=domain.NewCharacter(0,in.Code,in.Name,in.Persona,in.Description,time.Time{},time.Time{})
    if err!=nil{return domain.Character{},badDomain(err)}
    created,err:=u.store.CreateCharacter(ctx,value)
    if err!=nil{return domain.Character{},fmt.Errorf("create character: %w",err)}
    return created,nil
}

// UpdateCharacter updates a character but keeps its stable code.
func (u *Usecase) UpdateCharacter(ctx context.Context,id int64,in CharacterInput)(domain.Character,error){
    existing,err:=u.FindCharacter(ctx,id)
    if err!=nil{return domain.Character{},err}
    value,err:=domain.NewCharacter(existing.ID,existing.Code,in.Name,in.Persona,in.Description,existing.CreatedAt,existing.UpdatedAt)
    if err!=nil{return domain.Character{},badDomain(err)}
    updated,err:=u.store.UpdateCharacter(ctx,value)
    if err!=nil{return domain.Character{},fmt.Errorf("update character: %w",err)}
    return updated,nil
}

// DeleteCharacter refuses to remove characters still referenced by product models.
func (u *Usecase) DeleteCharacter(ctx context.Context,id int64) error {
    if err:=u.ready(ctx);err!=nil{return err}
    if id<=0{return apperr.NewBadRequest("invalid character id")}
    if err:=u.store.DeleteCharacter(ctx,id);err!=nil{return fmt.Errorf("delete character: %w",err)}
    return nil
}

// ListDeviceTypes returns product models and their assigned character IDs.
func (u *Usecase) ListDeviceTypes(ctx context.Context)([]domain.DeviceType,error){
    if err:=u.ready(ctx);err!=nil{return nil,err}
    return u.store.ListDeviceTypes(ctx)
}

// FindDeviceType returns one model.
func (u *Usecase) FindDeviceType(ctx context.Context,id int64)(domain.DeviceType,error){
    if err:=u.ready(ctx);err!=nil{return domain.DeviceType{},err}
    if id<=0{return domain.DeviceType{},apperr.NewBadRequest("invalid device type id")}
    return u.store.FindDeviceType(ctx,id)
}

// CreateDeviceType creates a model which references an existing character.
func (u *Usecase) CreateDeviceType(ctx context.Context,in DeviceTypeInput)(domain.DeviceType,error){
    if err:=u.ready(ctx);err!=nil{return domain.DeviceType{},err}
    value,err:=domain.NewDeviceType(0,in.Code,in.Name,in.Description,in.CharacterID,in.HasCamera,in.HasMicrophone,in.HasSpeaker,time.Time{},time.Time{})
    if err!=nil{return domain.DeviceType{},badDomain(err)}
    if _,err:=u.store.FindCharacter(ctx,value.CharacterID);err!=nil{return domain.DeviceType{},fmt.Errorf("device type character: %w",err)}
    created,err:=u.store.CreateDeviceType(ctx,value)
    if err!=nil{return domain.DeviceType{},fmt.Errorf("create device type: %w",err)}
    return created,nil
}

// UpdateDeviceType changes a model's role and capabilities without changing its code.
func (u *Usecase) UpdateDeviceType(ctx context.Context,id int64,in DeviceTypeInput)(domain.DeviceType,error){
    existing,err:=u.FindDeviceType(ctx,id)
    if err!=nil{return domain.DeviceType{},err}
    value,err:=domain.NewDeviceType(existing.ID,existing.Code,in.Name,in.Description,in.CharacterID,in.HasCamera,in.HasMicrophone,in.HasSpeaker,existing.CreatedAt,existing.UpdatedAt)
    if err!=nil{return domain.DeviceType{},badDomain(err)}
    if _,err:=u.store.FindCharacter(ctx,value.CharacterID);err!=nil{return domain.DeviceType{},fmt.Errorf("device type character: %w",err)}
    updated,err:=u.store.UpdateDeviceType(ctx,value)
    if err!=nil{return domain.DeviceType{},fmt.Errorf("update device type: %w",err)}
    return updated,nil
}

// DeleteDeviceType removes a product model without deleting its shared character.
func (u *Usecase) DeleteDeviceType(ctx context.Context,id int64) error {
    if err:=u.ready(ctx);err!=nil{return err}
    if id<=0{return apperr.NewBadRequest("invalid device type id")}
    if err:=u.store.DeleteDeviceType(ctx,id);err!=nil{return fmt.Errorf("delete device type: %w",err)}
    return nil
}
