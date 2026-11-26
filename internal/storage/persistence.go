package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

func Open(path string) (*Store, error) {
	s := &Store{path: path}
	normalizeState(&s.data)
	s.committed, _ = json.Marshal(s.data)
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(contents, &s.data); err != nil {
		bakPath := path + ".bak"
		bakContents, bakErr := os.ReadFile(bakPath)
		if bakErr == nil {
			var recovered State
			if json.Unmarshal(bakContents, &recovered) == nil {
				corruptPath := fmt.Sprintf("%s.corrupt.%d", path, time.Now().Unix())
				_ = os.Rename(path, corruptPath)
				slog.Warn("state file corrupt; recovered from backup", "path", path, "corrupt_backup", corruptPath, "error", err)
				s.data = recovered
				normalizeState(&s.data)
				s.committed, _ = json.Marshal(s.data)
				if persistErr := s.persistLocked(); persistErr != nil {
					return nil, fmt.Errorf("restore state from backup: %w", persistErr)
				}
				return s, nil
			}
		}
		return nil, fmt.Errorf("decode state: %w", err)
	}
	normalizeState(&s.data)
	s.committed, _ = json.Marshal(s.data)
	return s, nil
}

func writeDurableFile(targetPath string, contents []byte) error {
	dir := filepath.Dir(targetPath)
	tmpFile, err := os.CreateTemp(dir, "."+filepath.Base(targetPath)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if _, err := tmpFile.Write(contents); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	cleanup = false
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

func (s *Store) persistLocked() (err error) {
	// Failed writes must not commit the mutation only in RAM.
	defer func() {
		if err != nil {
			var previous State
			if json.Unmarshal(s.committed, &previous) == nil {
				s.data = previous
				normalizeState(&s.data)
			}
		}
	}()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(s.path); statErr == nil && len(s.committed) > 0 {
		_ = writeDurableFile(s.path+".bak", s.committed)
	}
	if err := writeDurableFile(s.path, contents); err != nil {
		return err
	}
	s.committed = append([]byte(nil), contents...)
	return nil
}

func normalizeState(state *State) {
	if state.MemberCredentials == nil {
		state.MemberCredentials = make(map[string]MemberCredential)
	}
	if state.Networks == nil {
		state.Networks = make(map[string]Network)
	}
	if state.Devices == nil {
		state.Devices = make(map[string]RegisteredDevice)
	}
	if state.Candidates == nil {
		state.Candidates = make(map[string]CandidateRecord)
	}
	if state.Diagnostics == nil {
		state.Diagnostics = make(map[string][]DiagnosticReport)
	}
	for id, n := range state.Networks {
		if n.Devices == nil {
			n.Devices = make(map[string]Device)
			state.Networks[id] = n
		}
	}
}
