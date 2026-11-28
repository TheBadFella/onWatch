package store

import (
	"encoding/json"
	"time"
)

// StarterStatus keeps the last attempt and the current reason a request is waiting.
type StarterStatus struct {
	Quota       string     `json:"quota"`
	Result      string     `json:"result,omitempty"`
	AttemptedAt *time.Time `json:"attemptedAt,omitempty"`
	ConfirmedAt *time.Time `json:"confirmedAt,omitempty"`
	Waiting     string     `json:"waiting,omitempty"`
	NextAt      *time.Time `json:"nextAt,omitempty"`
}

func (s *Store) GetStarterStatus(provider, account, quota string) (StarterStatus, error) {
	status := StarterStatus{Quota: quota}
	raw, err := s.GetSetting("starter_status:" + provider + ":" + account + ":" + quota)
	if err == nil && raw != "" {
		err = json.Unmarshal([]byte(raw), &status)
	}
	return status, err
}

func (s *Store) SetStarterStatus(provider, account string, status StarterStatus) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return s.SetSetting("starter_status:"+provider+":"+account+":"+status.Quota, string(raw))
}
