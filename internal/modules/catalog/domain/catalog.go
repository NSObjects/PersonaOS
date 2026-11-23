// Package domain defines character and device-type invariants.
package domain

import (
    "errors"
    "regexp"
    "strings"
    "time"
)

var (
    // ErrInvalidCharacter indicates invalid character metadata.
    ErrInvalidCharacter = errors.New("invalid character")
    // ErrInvalidDeviceType indicates invalid device-type metadata.
    ErrInvalidDeviceType = errors.New("invalid device type")
)

var codePattern = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9_-]{1,63}$")

// Character is a reusable persona, independent from physical device types.
type Character struct {
    ID          int64     `json:"id"`
    Code        string    `json:"code"`
    Name        string    `json:"name"`
    Persona     string    `json:"persona"`
    Description string    `json:"description"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// NewCharacter validates and normalizes a character.
func NewCharacter(id int64, code, name, persona, description string, created, updated time.Time) (Character, error) {
    code = strings.ToLower(strings.TrimSpace(code))
    name = strings.TrimSpace(name)
    persona = strings.TrimSpace(persona)
    description = strings.TrimSpace(description)
    if id < 0 || !codePattern.MatchString(code) || name == "" || len(name) > 160 ||
        persona == "" || len(persona) > 16000 || len(description) > 2000 {
        return Character{}, ErrInvalidCharacter
    }
    return Character{ID:id, Code:code, Name:name, Persona:persona, Description:description, CreatedAt:created, UpdatedAt:updated}, nil
}

// DeviceType defines one product model and its physical input/output features.
type DeviceType struct {
    ID            int64     `json:"id"`
    Code          string    `json:"code"`
    Name          string    `json:"name"`
    Description   string    `json:"description"`
    CharacterID   int64     `json:"character_id"`
    HasCamera     bool      `json:"has_camera"`
    HasMicrophone bool      `json:"has_microphone"`
    HasSpeaker    bool      `json:"has_speaker"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}

// NewDeviceType validates a product model. Camera capability is hardware metadata,
// not permission to use the observe_scene tool (which D2 will configure separately).
func NewDeviceType(id int64, code, name, description string, characterID int64, camera, microphone, speaker bool, created, updated time.Time) (DeviceType, error) {
    code = strings.ToUpper(strings.TrimSpace(code))
    name = strings.TrimSpace(name)
    description = strings.TrimSpace(description)
    if id < 0 || !codePattern.MatchString(code) || name == "" || len(name) > 160 ||
        len(description) > 2000 || characterID <= 0 {
        return DeviceType{}, ErrInvalidDeviceType
    }
    return DeviceType{ID:id, Code:code, Name:name, Description:description, CharacterID:characterID,
        HasCamera:camera, HasMicrophone:microphone, HasSpeaker:speaker, CreatedAt:created, UpdatedAt:updated}, nil
}
