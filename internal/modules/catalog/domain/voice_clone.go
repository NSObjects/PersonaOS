package domain

// Seed3CloneProvider identifies a registered Seed-TTS 3.0 clone, not a preset.
const Seed3CloneProvider = "volcengine_seed3_clone"

// VoiceCloneStatus describes the provider's current training result.
type VoiceCloneStatus struct {
	SpeakerID     string `json:"speaker_id"`
	State         int    `json:"status"`
	TrainingTimes int    `json:"available_training_times"`
	DemoAudio     string `json:"demo_audio,omitempty"`
	ModelReady    bool   `json:"model_ready"`
}
