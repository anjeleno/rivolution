package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// TaskType is which built-in behavior a ScheduledTask runs. See
// BACKLOG.md's "Dashboard needs to replace what the removed Ansible
// broadcast_advanced role did" entry -- this is that deferred work.
type TaskType string

const (
	TaskDBBackup     TaskType = "db_backup"
	TaskLogGen       TaskType = "log_gen"
	TaskLogReconcile TaskType = "log_reconcile"
	TaskCustom       TaskType = "custom"
)

// ScheduledTask is one operator-configured periodic job, backed by its own
// systemd service+timer pair (see tasks_deploy.go). ID is server-generated
// (see NewTaskID), never taken from user input, since it becomes part of a
// real filesystem path and systemd unit name -- see idRe's validation,
// checked before ID is ever used in a path.
type ScheduledTask struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Type    TaskType `json:"type"`
	Enabled bool     `json:"enabled"`

	// Schedule is a systemd OnCalendar= expression (e.g. "*-*-* 00:05:00"
	// for daily at 00:05). Passed through as-is to the generated .timer
	// unit -- systemd itself validates the syntax at daemon-reload time.
	Schedule string `json:"schedule"`

	// db_backup fields.
	BackupDir     string `json:"backup_dir,omitempty"`
	RetentionDays int    `json:"retention_days,omitempty"`

	// log_gen / log_reconcile fields.
	ServiceName string `json:"service_name,omitempty"`
	DaysOffset  int    `json:"days_offset,omitempty"`

	// custom fields.
	CustomCommand string `json:"custom_command,omitempty"`
}

// TasksConfigPath is where the scheduled-tasks list is persisted, same
// pattern as BroadcastConfigPath/ModeConfigPath.
const TasksConfigPath = "/home/rd/etc/rivolution/tasks.json"

// idRe is the only shape a task ID is ever allowed to have -- checked
// before ID appears in any filesystem path or systemd unit name, whether
// it came from NewTaskID (which only ever produces this shape) or was
// loaded back from tasks.json (defense in depth against a hand-edited or
// otherwise corrupted file).
var idRe = regexp.MustCompile(`^[a-f0-9]{16}$`)

// NewTaskID generates a fresh, safe task ID -- 16 hex characters, never
// derived from the operator-entered task Name, so a Name containing path
// separators or shell metacharacters can never influence a real file path.
func NewTaskID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func validTaskID(id string) bool {
	return idRe.MatchString(id)
}

// LoadTasks reads the saved task list. Returns an empty slice, not an
// error, if the file doesn't exist yet (no tasks configured).
func LoadTasks(path string) ([]ScheduledTask, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tasks []ScheduledTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// SaveTasks writes the full task list. Atomic (temp file + rename), same
// pattern as every other Save* in this package.
func SaveTasks(tasks []ScheduledTask, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// FindTask returns the task with the given ID, and whether it was found.
func FindTask(tasks []ScheduledTask, id string) (ScheduledTask, bool) {
	for _, t := range tasks {
		if t.ID == id {
			return t, true
		}
	}
	return ScheduledTask{}, false
}

// RemoveTaskFromList returns tasks with the given ID removed.
func RemoveTaskFromList(tasks []ScheduledTask, id string) []ScheduledTask {
	out := tasks[:0]
	for _, t := range tasks {
		if t.ID != id {
			out = append(out, t)
		}
	}
	return out
}

// TaskUnitName returns the systemd unit basename (without .service/.timer)
// for a task. Panics if id isn't already validated -- every caller is
// expected to have checked validTaskID first, since this is the one place
// an ID turns into a real systemd unit name.
func TaskUnitName(id string) string {
	if !validTaskID(id) {
		panic(fmt.Sprintf("internal error: invalid task ID %q reached TaskUnitName", id))
	}
	return "rivolution-task-" + id
}
